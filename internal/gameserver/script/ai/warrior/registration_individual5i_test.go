package warrior

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// individual5i are the classes of the fifth individual behavior batch's
// ninth PR (the aggressive runner, the runners to their clan and the
// slowing Warriors), in scripts.xml order, with the Go constructor of each.
var individual5i = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecial.WarriorRunAwayPhysicalSpecialAggressive", RunAwayPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayToClan.WarriorRunAwayToClan", RunAwayToClan},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayToClan.WarriorRunAwayToClanAggressive", RunAwayToClanAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType", SlowType},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType1", SlowType1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType1Aggressive", SlowType1Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType2", SlowType2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorSlowType.WarriorSlowType2Aggressive", SlowType2Aggressive},
}

// TestIndividual5iRegistrationMatchesReferenceManifest registers the
// ninth PR's classes after the behavior chain and the runner they derive
// from, through the engine registry with its own seam gate and every id a
// hostile NPC. None is refused, and the registry dump equals the reference
// manifest for them: each class's kind and bind lines, and every folded
// per-(npc, event) list restricted to them: the 60 ids on the Warrior
// chain's 10 events, and the 16 runners to their clan with move-finished.
func TestIndividual5iRegistrationMatchesReferenceManifest(t *testing.T) {
	parents := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecial.WarriorRunAwayPhysicalSpecial", RunAwayPhysicalSpecial},
	}
	list := make([]script.Listing, 0, len(chain)+len(parents)+len(individual5i))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range append(slices.Clone(chain), parents...) {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5i {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
		ours[c.path] = true
	}
	hostile := func(int32) (script.NPCKind, bool) { return script.KindHostile, true }
	r := script.Build(list, catalog, script.Config{KindOf: hostile, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := readBatchView(t, "dump", ours, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readBatchView(t, path, ours, sc)
	if wantFolds, got := 60*10+16, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5i {
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
		t.Errorf("folded lists differ: %d entries, reference %d", len(got.folds), len(want.folds))
	}
	for _, id := range []int32{20058, 20207, 20439, 20208, 20437, 20210, 20436, 20499, 20500, 20498, 20494, 20211, 20438, 20061, 20497, 20495, 20101, 20378, 20379, 20380, 20526, 21128, 21131, 20346, 20566, 20565, 20083, 20368, 20333, 21103, 20511, 20559, 20016, 20591, 20597, 20521, 21385, 21386, 20085, 20149, 21137, 20201, 20659, 21800, 20015, 20020, 27099, 27017, 20026, 20029, 20562, 20046, 20041, 27024, 20341, 20616, 20457, 22045, 20996, 20458} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
