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

// individual5f are the classes of the fifth individual behavior batch's
// sixth PR (the summon, lab clan and heal casters), in scripts.xml order,
// with the Go constructor of each.
var individual5f = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSummonPC.WarriorCastSummonPC", CastSummonPC},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSummonPC.WarriorCastSummonPCAggressive", CastSummonPCAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan1", CastingEnchantLabClan1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorCastingEnchantLabClan2", CastingEnchantLabClan2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorPhysicalSpecialLabAggressive", PhysicalSpecialLabAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorPhysicalSpecialLabRunAggressive", PhysicalSpecialLabRunAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHeal", CastingHeal},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHealAggressive", CastingHealAggressive},
}

// TestIndividual5fRegistrationMatchesReferenceManifest registers the sixth
// PR's classes after the behavior chain they derive from, through the
// engine registry with its own seam gate and every id a hostile NPC. None
// is refused, and the registry dump equals the reference manifest for
// them: each class's kind and bind lines, and every folded per-(npc,
// event) list restricted to them: the 11 ids on the Warrior chain's 10
// events.
func TestIndividual5fRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual5f))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5f {
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
	if wantFolds, got := 11*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5f {
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
	for _, id := range []int32{20213, 20221, 27318, 22059, 22064, 22065, 22076, 22077, 22078, 22063, 20686} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
