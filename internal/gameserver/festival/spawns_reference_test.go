package festival

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// referenceManager reads the reference festival manager's source, found
// beside the datapack checkout.
func referenceManager(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(datapack.Require(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(dir), "aCis_gameserver", "java", "net", "sf", "l2j", "gameserver", "data", "manager", "FestivalOfDarknessManager.java")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the reference festival manager: %v", err)
	}
	return string(raw)
}

var (
	javaComment  = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)
	javaDiff     = regexp.MustCompile(`(-?\d+)\s*-\s*(\d+)`)
	javaTokens   = regexp.MustCompile(`\{|\}|-?\d+`)
	javaTableEnd = "};"
)

// referenceTable parses the reference's int array name, nested braces
// becoming nested slices; "a - b" literals are folded.
func referenceTable(t *testing.T, src, name string) []any {
	t.Helper()
	start := strings.Index(src, name+" =")
	if start < 0 {
		t.Fatalf("no table %s", name)
	}
	body := src[start:]
	body = body[strings.Index(body, "{"):]
	body = body[:strings.Index(body, javaTableEnd)+1]
	body = javaComment.ReplaceAllString(body, "")
	body = javaDiff.ReplaceAllStringFunc(body, func(m string) string {
		p := javaDiff.FindStringSubmatch(m)
		a, _ := strconv.Atoi(p[1])
		b, _ := strconv.Atoi(p[2])
		return strconv.Itoa(a - b)
	})
	stack := [][]any{{}}
	for _, tok := range javaTokens.FindAllString(body, -1) {
		switch tok {
		case "{":
			stack = append(stack, []any{})
		case "}":
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stack[len(stack)-1] = append(stack[len(stack)-1], top)
		default:
			n, _ := strconv.Atoi(tok)
			stack[len(stack)-1] = append(stack[len(stack)-1], n)
		}
	}
	return stack[0][0].([]any)
}

func ints(t *testing.T, v any) []int {
	t.Helper()
	raw := v.([]any)
	out := make([]int, len(raw))
	for i, x := range raw {
		out[i] = x.(int)
	}
	return out
}

// The arenas' spawn tables are the reference's, entry for entry: each
// player start and witch, then each arena's first wave, archers and chests,
// Dusk's tables in the first side and Dawn's in the second. Every monster
// placed is a FestivalMonster of the shipped NPC data, and every witch a
// FestivalGuide.
func TestSpawnTablesMatchTheReference(t *testing.T) {
	src := referenceManager(t)
	for i, cabal := range []string{"DUSK", "DAWN"} {
		for id, row := range referenceTable(t, src, "FESTIVAL_"+cabal+"_PLAYER_SPAWNS") {
			e := ints(t, row)
			if got := playerSpawns[i][id]; got != (Spot{X: e[0], Y: e[1], Z: e[2], Heading: e[3]}) {
				t.Errorf("%s player spawn %d = %+v, want %v", cabal, id, got, e)
			}
		}
		for id, row := range referenceTable(t, src, "FESTIVAL_"+cabal+"_WITCH_SPAWNS") {
			e := ints(t, row)
			if got := witchSpawns[i][id]; got != (witchSpawn{npcID: e[4], x: e[0], y: e[1], z: e[2], heading: e[3]}) {
				t.Errorf("%s witch %d = %+v, want %v", cabal, id, got, e)
			}
		}
		for _, table := range []struct {
			name string
			got  [Count][]monsterSpawn
		}{{"PRIMARY", primarySpawns[i]}, {"SECONDARY", secondarySpawns[i]}, {"CHEST", chestSpawns[i]}} {
			for id, festival := range referenceTable(t, src, "FESTIVAL_"+cabal+"_"+table.name+"_SPAWNS") {
				rows := festival.([]any)
				if len(rows) != len(table.got[id]) {
					t.Errorf("%s %s %d: %d spawns, want %d", cabal, table.name, id, len(table.got[id]), len(rows))
					continue
				}
				for j, row := range rows {
					e := ints(t, row)
					if e[3] != -1 {
						t.Errorf("%s %s %d/%d heading %d, want a random one", cabal, table.name, id, j, e[3])
					}
					if got := table.got[id][j]; got != (monsterSpawn{npcID: e[4], x: e[0], y: e[1], z: e[2]}) {
						t.Errorf("%s %s %d/%d = %+v, want %v", cabal, table.name, id, j, got, e)
					}
				}
			}
		}
	}

	types := shippedNPCTypes(t)
	for i := range 2 {
		for id := range Count {
			if w := witchSpawns[i][id]; types[w.npcID] != "FestivalGuide" {
				t.Errorf("witch %d is a %q", w.npcID, types[w.npcID])
			}
			for _, table := range [][]monsterSpawn{primarySpawns[i][id], secondarySpawns[i][id], chestSpawns[i][id]} {
				for _, s := range table {
					if types[s.npcID] != "FestivalMonster" {
						t.Errorf("monster %d is a %q", s.npcID, types[s.npcID])
					}
				}
			}
		}
	}
}

// shippedNPCTypes reads each shipped NPC's instance type.
func shippedNPCTypes(t *testing.T) map[int]string {
	t.Helper()
	files, err := filepath.Glob(datapack.Path(t, "data", "xml", "npcs", "*.xml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob npc files: %v (%d)", err, len(files))
	}
	npcType := regexp.MustCompile(`<npc id="(\d+)"[^>]*>\s*<set name="type" val="([A-Za-z]+)"/>`)
	out := map[int]string{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range npcType.FindAllStringSubmatch(string(raw), -1) {
			id, _ := strconv.Atoi(m[1])
			out[id] = m[2]
		}
	}
	return out
}

// An archer is a festival monster numbered 18009 to 18108 ending in 4 or
// 9.
func TestArcher(t *testing.T) {
	for id, want := range map[int]bool{18009: true, 18014: true, 18019: true, 18104: true, 18010: false, 18008: false, 18109: false, 18119: false, 18011: false} {
		if got := archer(id); got != want {
			t.Errorf("archer(%d) = %v, want %v", id, got, want)
		}
	}
}
