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

// individual4 are the classes of the fourth individual behavior batch
// (#189) this package ports, in scripts.xml order, with the Go constructor
// of each.
var individual4 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.FreyaBear", FreyaBear},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.OrfenPrivateRaikelLeos", OrfenPrivateRaikelLeos},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCannibaliAggressive", PartyPrivateCannibaliAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCast3SkillsMagical", PartyPrivateCast3SkillsMagical},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastCurse", PartyPrivateCastCurse},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDHeal", PartyPrivateCastDDHeal},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDMagicCurseStone", PartyPrivateCastDDMagicCurseStone},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateHereticCastCurse", PartyPrivateHereticCastCurse},
}

// TestIndividual4RegistrationMatchesReferenceManifest registers the batch
// after the behavior chain it derives from, through the engine registry
// with its own seam gate and every id a hostile NPC (the datapack types all
// 16 ids Monster). None is refused, and the registry dump equals the
// reference manifest for the batch: each class's kind and bind lines (none
// for PartyPrivateCast3SkillsMagical, which has no NPC of its own), and
// every folded per-(npc, event) list restricted to the batch: the 11
// events of the PartyPrivateWarrior chain for most ids, plus PARTY_DIED
// for OrfenPrivateRaikelLeos and OUT_OF_TERRITORY for
// PartyPrivateHereticCastCurse.
func TestIndividual4RegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual4))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual4 {
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
	// FreyaBear, PartyPrivateCannibaliAggressive, PartyPrivateCastCurse,
	// PartyPrivateCastDDHeal (9) and PartyPrivateCastDDMagicCurseStone (2)
	// on 11 events; OrfenPrivateRaikelLeos and PartyPrivateHereticCastCurse
	// on 12.
	if wantFolds, got := (1+1+1+9+2)*11+2*12, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual4 {
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
	for _, id := range []int32{22103, 29016, 22112, 20982, 22157, 20948, 20943, 20987, 20975, 20764, 20981, 21059, 21074, 21080, 21349, 21375} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
