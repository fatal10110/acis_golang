package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// levelGate is a minimal basefunc.Condition mirroring skill/effect's
// conditionGate: it resolves effected (falling back to effector) to a
// conditions.Actor and requires its Level() to meet min. Before #1509,
// hostileStatActor didn't implement conditions.Actor, so this always
// resolved to false regardless of min — a conditional stat func on an
// NPC-owned skill silently never applied.
type levelGate struct{ min int }

func (g levelGate) Test(effector, effected, skill any) bool {
	actor, ok := effected.(conditions.Actor)
	if !ok {
		actor, ok = effector.(conditions.Actor)
	}
	return ok && actor.Level() >= g.min
}

func TestHostileConditionalStatFuncGatesOnRealLevel(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{
			ID:    9001,
			Type:  "Monster",
			Level: 15,
		},
		Kind: "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	// HealEffectiveness carries no default NPC stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	hostile.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(nil, stat.HealEffectiveness, 25, levelGate{min: 10}),
	})
	hostile.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(nil, stat.RechargeMPRate, 25, levelGate{min: 100}),
	})

	if got := hostile.CalcStat(stat.HealEffectiveness, 100); got != 125 {
		t.Errorf("CalcStat(HealEffectiveness) = %v, want 125 (level 15 >= 10 gate should pass)", got)
	}
	if got := hostile.CalcStat(stat.RechargeMPRate, 100); got != 100 {
		t.Errorf("CalcStat(RechargeMPRate) = %v, want 100 unchanged (level 15 >= 100 gate should fail)", got)
	}
}

func TestHostileStatActorImplementsConditionsActor(t *testing.T) {
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Monster", Level: 20, HPMax: 1000},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}

	var actor conditions.Actor = hostileStatActor{h: hostile}
	if actor.Level() != 20 {
		t.Errorf("Level() = %v, want 20", actor.Level())
	}
	if actor.HPRatio() <= 0 {
		t.Errorf("HPRatio() = %v, want > 0 for a freshly spawned NPC", actor.HPRatio())
	}
	if !actor.IsRunning() {
		t.Error("IsRunning() = false, want true (NPCs always spawn in run stance)")
	}
	if actor.IsRiding() || actor.IsFlying() {
		t.Error("IsRiding()/IsFlying() should default false for an NPC")
	}
	if _, ok := actor.ActiveSkillLevel(1); ok {
		t.Error("ActiveSkillLevel(1) ok = true, want false for an NPC with no active effects")
	}
}
