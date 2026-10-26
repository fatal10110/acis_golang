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

// individual5h are the classes of the fifth individual behavior batch's
// eighth PR (the heretic zombie, the shooters, the priests and the
// runner), in scripts.xml order, with the Go constructor of each.
var individual5h = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorHereticCorpseZombiePhysicalSpecial", HereticCorpseZombiePhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotNoRange", PhysicalSpecialShotNoRange},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotRange", PhysicalSpecialShotRange},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotRangeAggressive", PhysicalSpecialShotRangeAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecialShotRange.WarriorPhysicalSpecialShotRangeDebuffAggressive", PhysicalSpecialShotRangeDebuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPriest.WarriorPriest", Priest},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPriest.WarriorPriestAggressive", PriestAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecial.WarriorRunAwayPhysicalSpecial", RunAwayPhysicalSpecial},
}

// TestIndividual5hRegistrationMatchesReferenceManifest registers the
// eighth PR's classes after the behavior chain and the zombies they derive
// from, through the engine registry with its own seam gate and every id a
// hostile NPC. None is refused, and the registry dump equals the reference
// manifest for them: each class's kind and bind lines, and every folded
// per-(npc, event) list restricted to them: the 16 ids on the Warrior
// chain's 10 events, and the 2 heretic zombies with out-of-territory and
// the 6 runners with move-finished on 11.
func TestIndividual5hRegistrationMatchesReferenceManifest(t *testing.T) {
	parents := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombie", CorpseZombie},
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombiePhysicalSpecial", CorpseZombiePhysicalSpecial},
	}
	list := make([]script.Listing, 0, len(chain)+len(parents)+len(individual5h))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range append(slices.Clone(chain), parents...) {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5h {
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
	if wantFolds, got := 16*10+8*11, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5h {
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
	for _, id := range []int32{22140, 22149, 22066, 22067, 22068, 22069, 22070, 22071, 22072, 22073, 22053, 22054, 20925, 20929, 20931, 21646, 20934, 21306, 21508, 21510, 21511, 21513, 21515, 21516} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
