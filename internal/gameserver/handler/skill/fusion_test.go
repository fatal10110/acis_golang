package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// battleForce is the FUSION-skillType caster skill (id 426), triggering
// Battle Force (id 5104) at its own level.
func battleForce(level int) modelskill.Definition {
	return modelskill.Definition{
		ID: 426, Level: level, SkillType: "FUSION",
		TriggeredID: 5104, TriggeredLevel: level,
	}
}

func battleForceDefs() continuousDefinitions {
	defs := continuousDefinitions{}
	for level := 1; level <= 3; level++ {
		defs[modelskill.Ref{ID: 5104, Level: level}] = modelskill.Definition{
			ID: 5104, Level: level, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Fusion", Time: 600}},
		}
	}
	return defs
}

func TestFusionHandlerAppliesTriggeredSkillFreshWhenTargetHasNone(t *testing.T) {
	target := newContinuousFake(1)
	registry := NewDefaultRegistryWithDefinitions(battleForceDefs())

	registry.Use(Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []any{target}})

	e := firstEffectByID(target.list, 5104)
	if e == nil {
		t.Fatal("no fusion effect applied to a target with no prior one")
	}
	if e.Level != 1 {
		t.Fatalf("fresh fusion effect Level = %d, want 1", e.Level)
	}
}

func TestFusionHandlerRecastGrowsExistingEffectInPlace(t *testing.T) {
	target := newContinuousFake(1)
	registry := NewDefaultRegistryWithDefinitions(battleForceDefs())
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []any{target}}

	registry.Use(cast)
	first := firstEffectByID(target.list, 5104)

	registry.Use(cast)
	second := firstEffectByID(target.list, 5104)

	if second == first {
		t.Fatal("IncreaseEffect removes and reapplies a fresh instance, not the same pointer")
	}
	if second == nil || second.Level != 2 {
		t.Fatalf("Level after recast growth = %v, want 2", second)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("effect list has %d effects, want exactly 1 (no duplicate)", len(target.list.All()))
	}
}

func TestFusionHandlerCapsGrowthAtMaxLevel(t *testing.T) {
	target := newContinuousFake(1)
	registry := NewDefaultRegistryWithDefinitions(battleForceDefs())
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []any{target}}

	registry.Use(cast) // level 1
	registry.Use(cast) // level 2
	registry.Use(cast) // level 3 (max)
	registry.Use(cast) // no-op past max

	e := firstEffectByID(target.list, 5104)
	if e == nil || e.Level != 3 {
		t.Fatalf("Level at/past cap = %v, want 3", e)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("effect list has %d effects, want exactly 1", len(target.list.All()))
	}
}

func TestDecreaseFusionShrinksTriggeredEffectWhenChannelEnds(t *testing.T) {
	target := newContinuousFake(1)
	defs := battleForceDefs()
	registry := NewDefaultRegistryWithDefinitions(defs)
	cast := Cast{Caster: newContinuousFake(2), Skill: battleForce(1), Targets: []any{target}}

	registry.Use(cast)
	registry.Use(cast)
	DecreaseFusion(defs, cast.Caster, target, cast.Skill)

	e := firstEffectByID(target.list, 5104)
	if e == nil || e.Level != 1 {
		t.Fatalf("fusion level after channel end = %v, want level 1", e)
	}
}
