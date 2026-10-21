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

// individual1e are the classes of the first individual behavior batch's
// fifth PR (#186), in scripts.xml order, with the Go constructor of each.
var individual1e = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproachRevivalAggressive", Cast3SkillsApproachRevivalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproachStoneAggressive", Cast3SkillsApproachStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsCurse", Cast3SkillsCurse},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsCurseAggressive", Cast3SkillsCurseAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagical", Cast3SkillsMagical},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagicalAggressive", Cast3SkillsMagicalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagicalRevived", Cast3SkillsMagicalRevived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.ZakenPrivateValeMaster", ZakenPrivateValeMaster},
}

// TestIndividual1eRegistrationMatchesReferenceManifest registers the
// fifth PR's classes after the behavior chain they derive from, through
// the engine registry with its own seam gate and every id a hostile NPC.
// None is refused, and the registry dump equals the reference manifest for
// them: each class's kind and bind lines, and every folded per-(npc, event)
// list restricted to them, each of the 37 ids on the 10 events of the
// Warrior chain.
func TestIndividual1eRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual1e))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual1e {
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
	if wantFolds, got := (6+4+1+1+12+10+2+1)*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual1e {
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
	for _, id := range []int32{20830, 21335, 21301, 21302, 20610, 20822, 20859, 29024} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
