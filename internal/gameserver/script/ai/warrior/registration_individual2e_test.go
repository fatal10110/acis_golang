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

// individual2e are the classes of the second individual behavior batch
// (#187) its fifth PR ports, in scripts.xml order, with the Go
// constructor of each.
var individual2e = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFleeFromClanAggressive", FleeFromClanAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPassiveUsePowershot", PassiveUsePowershot},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPatrolWeaponEquipped", PatrolWeaponEquipped},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialAggressive", PhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialAreaAggressive", PhysicalSpecialAreaAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialGatheringAggressive", PhysicalSpecialGatheringAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialPotionAggressive", PhysicalSpecialPotionAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialSpoilAggressive", PhysicalSpecialSpoilAggressive},
}

// TestIndividual2eRegistrationMatchesReferenceManifest registers the
// batch after the behavior chains it derives from, through the engine
// registry with its own seam gate and every id a hostile NPC. None is
// refused, and the registry dump equals the reference manifest for the
// batch: each class's kind and bind lines, and every folded per-(npc,
// event) list restricted to the batch: the 10 events of the Warrior chain
// for each id, and the clan died event as well for the 4 ids of
// WarriorFleeFromClanAggressive.
func TestIndividual2eRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual2e))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual2e {
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
	got := readIndividual1View(t, "dump", ours, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readIndividual1View(t, path, ours, sc)
	if wantFolds, got := 210*10+4*11, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual2e {
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
	for _, id := range []int32{21443, 20268, 22124, 21427, 22011, 21676, 22022, 22017} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
