package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func seedOfFire() modelskill.Definition {
	return modelskill.Definition{
		ID:        1285,
		Level:     1,
		SkillType: "SEED",
		Effects:   []modelskill.EffectTemplate{{Name: "Seed", Time: 5}},
	}
}

func TestSeedHandlerAppliesFreshEffectWhenTargetHasNone(t *testing.T) {
	target := newContinuousFake(1)
	cast := Cast{Caster: newContinuousFake(2), Skill: seedOfFire(), Targets: []any{target}}

	seedHandler{}.Use(cast)

	e := firstEffectByID(target.list, 1285)
	if e == nil {
		t.Fatal("no seed effect applied to a target with no prior seed")
	}
	if e.Level != 1 {
		t.Fatalf("fresh seed effect Level = %d, want 1", e.Level)
	}
}

func TestSeedHandlerRecastGrowsExistingEffectInPlaceInsteadOfDuplicating(t *testing.T) {
	target := newContinuousFake(1)
	cast := Cast{Caster: newContinuousFake(2), Skill: seedOfFire(), Targets: []any{target}}

	seedHandler{}.Use(cast)
	first := firstEffectByID(target.list, 1285)

	seedHandler{}.Use(cast)
	second := firstEffectByID(target.list, 1285)

	if second != first {
		t.Fatal("recasting the same seed skill must grow the existing instance, not replace it")
	}
	if second.Level != 2 {
		t.Fatalf("Level after recast = %d, want 2", second.Level)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("effect list has %d effects, want exactly 1 (no duplicate seed)", len(target.list.All()))
	}
}

// Recasting one seed skill must not disturb an unrelated seed already
// active on the same target — no reschedule, no level change. That the
// recast also leaves the recast seed's own deadline unmoved is verified in
// the effect package's own tests, which have access to the unexported
// schedule state.
func TestSeedHandlerRecastLeavesOtherActiveSeedsInPlace(t *testing.T) {
	target := newContinuousFake(1)
	fire := Cast{Caster: newContinuousFake(2), Skill: seedOfFire(), Targets: []any{target}}
	water := Cast{Caster: fire.Caster, Skill: modelskill.Definition{
		ID: 1286, Level: 1, SkillType: "SEED",
		Effects: []modelskill.EffectTemplate{{Name: "Seed", Time: 5}},
	}, Targets: []any{target}}

	seedHandler{}.Use(fire)
	seedHandler{}.Use(water)
	seedHandler{}.Use(fire)

	waterEffect := firstEffectByID(target.list, 1286)
	if waterEffect == nil {
		t.Fatal("recasting fire must not remove the unrelated water seed")
	}
	if waterEffect.Level != 1 {
		t.Fatalf("water seed Level = %d, want unchanged 1", waterEffect.Level)
	}
}
