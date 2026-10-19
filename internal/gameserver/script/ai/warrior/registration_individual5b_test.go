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

// individual5b are the classes of the fifth individual behavior batch's
// second PR (the bombs, WarriorCastEnchant1Of4 and the corpse ghosts), in
// scripts.xml order, with the Go constructor of each.
var individual5b = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBomb", Bomb},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBombAggressive", BombAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTeleportBombAggressive", TeleportBombAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchant1Of4", CastEnchant1Of4},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchant1Of4Aggressive", CastEnchant1Of4Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostPhysicalSpecial", CorpseGhostPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostPhysicalSpecialAggressive", CorpseGhostPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushBomb", RoyalRushBomb},
}

// TestIndividual5bRegistrationMatchesReferenceManifest registers the
// second PR's classes after the behavior chain they derive from, through
// the engine registry with its own seam gate and every id a hostile NPC.
// None is refused, and the registry dump equals the reference manifest for
// them: each class's kind and bind lines, and every folded per-(npc, event)
// list restricted to them, each of the 6 + 2 + 1 + 1 + 4 + 1 + 1 + 3 ids
// on the 10 events of the Warrior chain.
func TestIndividual5bRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual5b))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5b {
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
	if wantFolds, got := (6+2+1+1+4+1+1+3)*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5b {
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
	for _, id := range []int32{21666, 18333, 22133, 22009, 21426, 21555, 21571, 18149} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
