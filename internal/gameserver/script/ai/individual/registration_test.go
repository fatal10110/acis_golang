package individual

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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group"
	"github.com/rs/zerolog"
)

// listed are the scripts.xml entries from Walkers to WarriorPetForPC,
// in file order, with the Go constructor of each; ported marks the ones
// this package ports.
var listed = []struct {
	path   string
	ctor   func() script.Script
	ported bool
}{
	{"script.ai.group.Walkers", group.Walkers, false},
	{"script.ai.individual.AltarDoorController", AltarDoorController, true},
	{"script.ai.individual.AreadataSwitch", AreadataSwitch, true},
	{"script.ai.individual.Corpse", Corpse, true},
	{"script.ai.individual.DaimonTheWhiteEyed", DaimonTheWhiteEyed, true},
	{"script.ai.individual.DefaultNpc", defaultnpc.New, false},
	{"script.ai.individual.ImperialGravekeeper", ImperialGravekeeper, true},
	{"script.ai.individual.RoyalRushMissionNpc", RoyalRushMissionNpc, true},
	{"script.ai.individual.RoyalRushNpc", RoyalRushNpc, true},
	{"script.ai.individual.SacrificialVictim", SacrificialVictim, true},
	{"script.ai.individual.TriolCamera", TriolCamera, true},
	{"script.ai.individual.WarriorPetForPC", WarriorPetForPC, true},
}

// folkIDs are the bound ids the datapack types Folk; the rest are Monster.
func folkIDs() []int32 {
	return []int32{13015, 31705, 31921, 31922, 31923, 31924, 32038, 32051, 32058, 32059, 32060, 32061, 32062, 32063, 32064, 32065, 32066, 32067, 32068}
}

// registrationView is what a manifest-layout text records of the ported
// classes: per class its kind and bind lines, and the folded per-(npc,
// event) lists that name one of them, as "event id paths" triples with
// every listed path kept.
type registrationView struct {
	kinds map[string]string
	binds map[string][]string
	folds []string
}

func manifestIDs(t *testing.T, s string) []int {
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

func readRegistrationView(t *testing.T, path string, ours, kept map[string]bool, text *bufio.Scanner) registrationView {
	t.Helper()
	v := registrationView{kinds: map[string]string{}, binds: map[string][]string{}}
	var cur string
	for text.Scan() {
		line := text.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = fields[1]
		case strings.HasPrefix(line, "  kind ") && ours[cur]:
			v.kinds[cur] = fields[1]
		case strings.HasPrefix(line, "  bind ") && ours[cur]:
			v.binds[cur] = append(v.binds[cur], strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc "):
			paths := strings.Split(fields[3], ",")
			if !slices.ContainsFunc(paths, func(p string) bool { return ours[p] }) {
				continue
			}
			paths = slices.DeleteFunc(paths, func(p string) bool { return !kept[p] })
			for _, ev := range strings.Split(fields[2], ",") {
				for _, id := range manifestIDs(t, fields[4]) {
					v.folds = append(v.folds, ev+" "+strconv.Itoa(id)+" "+strings.Join(paths, ","))
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

// TestRegistrationMatchesReferenceManifest registers the classes with
// Walkers and DefaultNpc, in scripts.xml order, through the engine registry
// and its seam gate, each id of the kind the datapack gives it. None is
// refused, and the registry dump equals the reference manifest: each
// class's kind and bind lines (DaimonTheWhiteEyed a plain script binding
// created and first talk), and the folded per-(npc, event) lists naming
// one of them: 122 entries, Daimon's created list after Walkers'.
func TestRegistrationMatchesReferenceManifest(t *testing.T) {
	var list []script.Listing
	catalog := script.Catalog{}
	ours, kept := map[string]bool{}, map[string]bool{}
	for _, c := range listed {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
		ours[c.path] = c.ported
		kept[c.path] = true
	}
	kindOf := func(id int32) (script.NPCKind, bool) {
		if slices.Contains(folkIDs(), id) {
			return script.KindFolk, true
		}
		return script.KindHostile, true
	}
	r := script.Build(list, catalog, script.Config{KindOf: kindOf, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := readRegistrationView(t, "dump", ours, kept, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readRegistrationView(t, path, ours, kept, sc)
	if n := len(want.folds); n != 122 {
		t.Fatalf("reference folds = %d entries, want 122", n)
	}

	for _, c := range listed {
		if !c.ported {
			continue
		}
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
	if !slices.Equal(got.folds, want.folds) {
		t.Errorf("folded lists differ:\n got %q\nwant %q", got.folds, want.folds)
	}
	for _, id := range []int32{32051, 32058, 18119, 27181, 18150, 31921, 32038, 13015, 16013} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a ported behavior bound", id)
		}
	}
	if r.Behaves(31705) {
		t.Error("Behaves(31705) = true, want no behavior: DaimonTheWhiteEyed is a plain script")
	}
}
