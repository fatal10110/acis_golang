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

// individual5d are the classes of the fifth individual behavior batch's
// fourth PR (the walking tyrannosaurus, the DD magic physical specials and
// the physical poisons), in scripts.xml order, with the Go constructor of
// each.
var individual5d = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimevalWalker", TyrannoPrimevalWalker},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecial", CastDDMagicPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialAggressive", CastDDMagicPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialBuffAggressive", CastDDMagicPhysicalSpecialBuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastDDMagicEnchantPhysicalPoisonAggressive", CastDDMagicEnchantPhysicalPoisonAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoison", CastEnchantPhysicalPoison},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoisonAggressive", CastEnchantPhysicalPoisonAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantRangePhysicalPoisonAggressive", CastEnchantRangePhysicalPoisonAggressive},
}

// TestIndividual5dRegistrationMatchesReferenceManifest registers the fourth
// PR's classes after the behavior chain they derive from, through the
// engine registry with its own seam gate and every id a hostile NPC. None
// is refused, and the registry dump equals the reference manifest for
// them: each class's kind and bind lines (WarriorCastEnchantPhysicalPoison
// has no ids and binds nothing), and every folded per-(npc, event) list
// restricted to them: the walking tyrannosaurus's id on 11 events
// (abnormal status changed added) and the 15 ids of the six others with
// ids on the Warrior chain's 10.
func TestIndividual5dRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual5d))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5d {
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
	if wantFolds, got := 1*11+15*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5d {
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
	for _, id := range []int32{22217, 22046, 22049, 21308, 21384, 21643, 21316, 21321, 21314, 21319} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
