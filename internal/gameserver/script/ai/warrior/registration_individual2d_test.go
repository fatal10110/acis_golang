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

// individual2d are the classes of the second individual behavior batch
// (#187) its fourth PR ports, in scripts.xml order, with the Go
// constructor of each.
var individual2d = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantSelfAggressive", CastingEnchantSelfAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.ZakenPrivateDollBlader", ZakenPrivateDollBlader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorPhysicalSpecial", RoyalRushWarriorPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorStone", RoyalRushWarriorStone},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.SaintNinja", SaintNinja},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampire", CorpseVampire},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireAggressive", CorpseVampireAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireMobHate", CorpseVampireMobHate},
}

// TestIndividual2dRegistrationMatchesReferenceManifest registers the
// batch after the behavior chains it derives from, through the engine
// registry with its own seam gate and every id a hostile NPC. None is
// refused, and the registry dump equals the reference manifest for the
// batch: each class's kind and bind lines, and every folded per-(npc,
// event) list restricted to the batch: the 10 events of the Warrior chain
// for each id.
func TestIndividual2dRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual2b)+len(individual2c)+len(individual2d))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	// The WarriorCastingEnchant subtree derives through the earlier
	// batches, which the chain does not list.
	for _, set := range [][]struct {
		path string
		ctor func() script.Script
	}{chain, individual2b, individual2c} {
		for _, c := range set {
			list = append(list, script.Listing{Path: c.path})
			catalog[c.path] = c.ctor
		}
	}
	for _, c := range individual2d {
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
	if wantFolds, got := 8*10+1*10+10*10+1*10+6*10+2*10+3*10+3*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual2d {
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
	for _, id := range []int32{20260, 20098, 29023, 18133, 21418, 18243, 21539, 21658, 21424, 21593, 21425, 21568, 21582, 21587} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
