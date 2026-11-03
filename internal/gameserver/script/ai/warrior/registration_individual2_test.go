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

// individual2 are the classes of the second individual behavior batch
// (#187) this package ports, in scripts.xml order, with the Go constructor
// of each.
var individual2 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.Agit02DoomServantAgit", Agit02DoomServantAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivatePerum", CorePrivatePerum},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivateWarriorCastDDMagicAggressive", CorePrivateCastDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagic", CastDDMagic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicAggressive", CastDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuff", CastDDMagicSelfBuff},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuffAggressive", CastDDMagicSelfBuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurse", CastingCurse},
}

// TestIndividual2RegistrationMatchesReferenceManifest registers the batch
// after the behavior chain it derives from, through the engine registry
// with its own seam gate and every id a hostile NPC (the datapack types all
// 117 ids Monster). None is refused, and the registry dump equals the
// reference manifest for the batch: each class's kind and bind lines (none
// for WarriorCastDDMagicSelfBuffAggressive, which has no NPC of its own),
// and every folded per-(npc, event) list restricted to the batch, each of
// the 117 ids on the 10 events of the Warrior chain.
func TestIndividual2RegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual2))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual2 {
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
	if wantFolds, got := (2+1+2+30+47+1+34)*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual2 {
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
	for _, id := range []int32{35632, 35411, 29012, 29008, 29011, 20037, 20599, 21421, 20412, 21405, 20075, 22051} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
