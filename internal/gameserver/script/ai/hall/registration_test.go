package hall

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/rs/zerolog"
)

// chain is the behavior chain down to the hall behaviors of this package,
// in scripts.xml order, with the Go constructor of each.
var chain = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.siegablehall.Agit01PartisanSorcerer1", PartisanSorcerer1},
	{"script.ai.siegablehall.Agit02Dietrich", Dietrich},
	{"script.ai.siegablehall.Agit02Gustav", Gustav},
	{"script.ai.siegablehall.Agit02Mikhail", Mikhail},
	{"script.ai.siegablehall.Agit03Alfred", Alfred},
	{"script.ai.siegablehall.Agit03Giselle", Giselle},
	{"script.ai.siegablehall.Agit03Lidia", Lidia},
	{"script.ai.siegablehall.AzitWateringGroud", Gourd},
	{"script.ai.siegablehall.AzitWateringMimic", Mimic},
	{"script.ai.siegablehall.AzitWateringMimicMagical", MimicMagical},
	{"script.ai.siegablehall.AzitWateringRecycleCan", RecycleCan},
	{"script.ai.siegablehall.AzitWateringTreasureBox", TreasureBox},
	{"script.ai.siegablehall.Agit01Move1.Agit01Move1", Move1},
	{"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2", PartisanSorcerer2},
	{"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1", PartisanHealer1},
	{"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManager", func() script.Script { return GameManager(NewWateringGame()) }},
}

// folkIDs are the chain's civilian NPCs, as the datapack types them: the
// Rainbow Springs gourds and sweeper. Every other id is a monster.
var folkIDs = []int32{35588, 35589, 35590, 35591, 35600}

// manifestView is what the reference manifest records of the chain: per
// script its kind and bind lines, and the folded per-(npc, event) lists
// restricted to the chain's scripts, as "event id paths" triples.
type manifestView struct {
	kinds map[string]string
	binds map[string][]string
	folds []string
}

// expandIDs reads a manifest id list: ids and a-b runs, comma-separated.
func expandIDs(t *testing.T, s string) []int {
	t.Helper()
	var out []int
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			t.Fatalf("bad id %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				t.Fatalf("bad id %q", part)
			}
		}
		for id := a; id <= b; id++ {
			out = append(out, id)
		}
	}
	return out
}

// readView reads a manifest-layout text (the reference manifest or a
// registry dump) as the chain sees it.
func readView(t *testing.T, path string, text *bufio.Scanner) manifestView {
	t.Helper()
	ours := map[string]bool{}
	for _, c := range chain {
		ours[c.path] = true
	}
	v := manifestView{kinds: map[string]string{}, binds: map[string][]string{}}
	var cur string
	for text.Scan() {
		line := text.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = fields[1]
			if ours[cur] && len(fields) > 2 {
				t.Fatalf("%s: %s", path, line)
			}
		case strings.HasPrefix(line, "  kind ") && ours[cur]:
			v.kinds[cur] = fields[1]
		case strings.HasPrefix(line, "  bind ") && ours[cur]:
			v.binds[cur] = append(v.binds[cur], strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc "):
			var kept []string
			for _, p := range strings.Split(fields[3], ",") {
				if ours[p] {
					kept = append(kept, p)
				}
			}
			if len(kept) == 0 {
				continue
			}
			for _, ev := range strings.Split(fields[2], ",") {
				for _, id := range expandIDs(t, fields[4]) {
					v.folds = append(v.folds, ev+" "+strconv.Itoa(id)+" "+strings.Join(kept, ","))
				}
			}
		}
	}
	if err := text.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	slices.Sort(v.folds)
	return v
}

// TestRegistrationMatchesReferenceManifest registers the chain's Go
// scripts through the engine registry, with the engine's own seam gate and
// each id of the kind the datapack gives it (folkIDs civilian, the rest
// monsters). None is refused, and the registry dump equals the reference
// manifest for the chain: each script's kind and bind lines, and every
// folded per-(npc, event) list restricted to the chain's scripts: the
// sorcerer's id on 2 events, Dietrich's on 5, Gustav's on 5, Mikhail's on 4,
// Alfred's and Giselle's on 3 each, Lidia's on 5, the four gourds' on 2
// each, the mimic's on 1, the magical mimic's on 2, the sweeper's on 4, the
// treasure box's on 3, the three soldiers' on 4 each, the second sorcerer's
// on 4 and the healer's on 1.
func TestRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, len(chain))
	catalog := script.Catalog{}
	for i, c := range chain {
		list[i] = script.Listing{Path: c.path}
		catalog[c.path] = c.ctor
	}
	kindOf := func(id int32) (script.NPCKind, bool) {
		if slices.Contains(folkIDs, id) {
			return script.KindFolk, true
		}
		return script.KindHostile, true
	}
	r := script.Build(list, catalog, script.Config{KindOf: kindOf, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := readView(t, "dump", bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readView(t, path, sc)

	for _, c := range chain {
		if want.kinds[c.path] == "" {
			t.Fatalf("%s is not in the reference manifest", c.path)
		}
		if got.kinds[c.path] != want.kinds[c.path] {
			t.Errorf("%s kind = %q, want %q", c.path, got.kinds[c.path], want.kinds[c.path])
		}
		if !slices.Equal(got.binds[c.path], want.binds[c.path]) {
			t.Errorf("%s binds:\n got %q\nwant %q", c.path, got.binds[c.path], want.binds[c.path])
		}
	}
	if wantFolds, got := 2+5+5+4+3+3+5+4*2+1+2+4+3+3*4+4+1, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}
	if !slices.Equal(got.folds, want.folds) {
		t.Errorf("folded lists differ: %d entries, reference %d", len(got.folds), len(want.folds))
	}
	for _, id := range []int32{35373, 35408, 35410, 35409, 35630, 35631, 35629, 35588, 35589, 35590, 35591, 35593, 35594, 35600, 35595, 35372, 35379, 35381, 35380, 35369} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
