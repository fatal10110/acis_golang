package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// levelGate is a minimal basefunc.Condition mirroring skill/effect's
// conditionGate: it resolves effected (falling back to effector) to a
// conditions.Actor and requires its Level() to meet min. Before #1509,
// summonStatActor didn't implement conditions.Actor, so this always
// resolved to false regardless of min — a conditional stat func on a
// summon-owned skill silently never applied.
type levelGate struct{ min int }

func (g levelGate) Test(effector, effected, skill any) bool {
	actor, ok := effected.(conditions.Actor)
	if !ok {
		actor, ok = effector.(conditions.Actor)
	}
	return ok && actor.Level() >= g.min
}

func TestSummonConditionalStatFuncGatesOnRealLevel(t *testing.T) {
	a := NewServitor(ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	// HealEffectiveness carries no default summon stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	a.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(nil, stat.HealEffectiveness, 25, levelGate{min: 10}),
	})
	a.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(nil, stat.RechargeMPRate, 25, levelGate{min: 100}),
	})

	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 125 {
		t.Errorf("CalcStat(HealEffectiveness) = %v, want 125 (level 44 >= 10 gate should pass)", got)
	}
	if got := a.CalcStat(stat.RechargeMPRate, 100); got != 100 {
		t.Errorf("CalcStat(RechargeMPRate) = %v, want 100 unchanged (level 44 >= 100 gate should fail)", got)
	}
}

func TestSummonStatActorImplementsConditionsActor(t *testing.T) {
	stats := CombatStats{MaxHP: 500, MaxMP: 200}
	a := NewServitor(ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	var actor conditions.Actor = summonStatActor{a: a}
	if actor.Level() != 44 {
		t.Errorf("Level() = %v, want 44", actor.Level())
	}
	if actor.HPRatio() <= 0 {
		t.Errorf("HPRatio() = %v, want > 0 for a freshly spawned summon", actor.HPRatio())
	}
	if !actor.IsRunning() {
		t.Error("IsRunning() = false, want true (non-player actors default to run stance)")
	}
	if actor.IsRiding() || actor.IsFlying() || actor.IsMoving() {
		t.Error("IsRiding()/IsFlying()/IsMoving() should default false for a summon")
	}
	if _, ok := actor.ActiveSkillLevel(1); ok {
		t.Error("ActiveSkillLevel(1) ok = true, want false for a summon with no active effects")
	}
}
