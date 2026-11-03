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

// individual2c are the classes of the second individual behavior batch
// (#187) its third PR ports, in scripts.xml order, with the Go
// constructor of each.
var individual2c = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.SpecialBloodyQueen", SpecialBloodyQueen},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgit", Agit02DoomArcherAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgitParty", Agit02DoomArcherAgitParty},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomWarriorAgit", Agit02DoomWarriorAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.OrfenPrivateRaikel", OrfenPrivateRaikel},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchant", CastingEnchant},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClan", CastingEnchantClan},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClanAggressive", CastingEnchantClanAggressive},
}

// TestIndividual2cRegistrationMatchesReferenceManifest registers the
// batch after the behavior chain it derives from, through the engine
// registry with its own seam gate and every id a hostile NPC (the datapack
// types all 61 ids Monster). None is refused, and the registry dump equals
// the reference manifest for the batch: each class's kind and bind lines,
// and every folded per-(npc, event) list restricted to the batch: the 10
// events of the Warrior chain, plus the door clan event for
// Agit02DoomArcherAgit and party attacked for Agit02DoomArcherAgitParty.
func TestIndividual2cRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual2c))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	// SpecialBloodyQueen derives from WarriorCastingCurseAggressive, which
	// the chain does not list.
	for _, c := range individual2b {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual2c {
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
	if wantFolds, got := 1*10+1*11+1*11+2*10+1*10+15*10+14*10+26*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual2c {
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
	for _, id := range []int32{21084, 35413, 35646, 35415, 35636, 29015, 20681, 22085, 21616, 20682, 21237, 21233} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
