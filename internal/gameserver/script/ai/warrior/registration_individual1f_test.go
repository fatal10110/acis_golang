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

// individual1f are the classes of the first individual behavior batch's
// sixth PR (#186), in scripts.xml order, with the Go constructor of each.
var individual1f = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2", Cast3SkillsMagical2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2Aggressive", Cast3SkillsMagical2Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2Revived", Cast3SkillsMagical2Revived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2StoneAggressive", Cast3SkillsMagical2StoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3", Cast3SkillsMagical3},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3Aggressive", Cast3SkillsMagical3Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3Revived", Cast3SkillsMagical3Revived},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical4.WarriorCast3SkillsMagical4", Cast3SkillsMagical4},
}

// TestIndividual1fRegistrationMatchesReferenceManifest registers the
// sixth PR's classes after the behavior chain they derive from, through
// the engine registry with its own seam gate and every id a hostile NPC.
// None is refused, and the registry dump equals the reference manifest for
// them: each class's kind and bind lines, and every folded per-(npc, event)
// list restricted to them, each of the 50 ids on the events of its chain.
func TestIndividual1fRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual1f))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual1f {
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
	if wantFolds, got := (18+17+2+2+3+4+1+3)*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual1f {
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
	for _, id := range []int32{20411, 18001, 20860, 21331, 20611, 20821, 21063, 21197} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
