package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// openGeo allows every straight-line move and echoes back requested
// heights/targets unmodified, enough to drive CreatureMove.MoveToLocation
// deterministically without real geodata.
type openGeo struct{}

func (openGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (openGeo) Height(x, y, z int) int16                { return int16(z) }
func (openGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (openGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// movingGate is a minimal effect.Condition mirroring skill/effect's
// conditionGate for a <player moving="true"/> tag: it resolves effector to
// a conditions.Actor and requires IsMoving(). Exercises #1510: before it,
// summonStatActor.IsMoving() was hardcoded false, so this gate could never
// pass regardless of real movement state.
type movingGate struct{}

func (g movingGate) Test(effector stat.Actor) bool {
	actor, ok := effector.(conditions.Actor)
	return ok && actor.IsMoving()
}

func TestSummonConditionalStatFuncGatesOnRealMovement(t *testing.T) {
	a := NewServitor(ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	// HealEffectiveness carries no default summon stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	a.AddStatFuncs([]effect.Mod{
		{Stat: stat.HealEffectiveness, Op: effect.OpAdd, Value: 25, Cond: movingGate{}},
	})

	if err := a.InitMovement(location.Location{X: 0, Y: 0, Z: 0}, 100, openGeo{}); err != nil {
		t.Fatalf("InitMovement: %v", err)
	}

	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 100 {
		t.Errorf("CalcStat(HealEffectiveness) before moving = %v, want 100 unchanged (gate should fail while still)", got)
	}

	if _, err := a.Move().MoveToLocation(location.Location{X: 1000, Y: 0, Z: 0}); err != nil {
		t.Fatalf("MoveToLocation: %v", err)
	}
	if !a.Move().Moving() {
		t.Fatal("Move().Moving() = false right after an accepted non-zero-distance move request")
	}
	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 125 {
		t.Errorf("CalcStat(HealEffectiveness) while moving = %v, want 125 (gate should pass while moving)", got)
	}

	a.Move().SetPosition(location.Location{X: 1000, Y: 0, Z: 0})
	if a.Move().Moving() {
		t.Fatal("Move().Moving() = true after SetPosition reports arrival at the destination")
	}
	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 100 {
		t.Errorf("CalcStat(HealEffectiveness) after stopping = %v, want 100 unchanged (gate should withdraw once movement stops)", got)
	}
}

// levelGate is a minimal effect.Condition mirroring skill/effect's
// conditionGate: it resolves effector to a conditions.Actor and requires
// its Level() to meet min. Before #1509, summonStatActor didn't implement
// conditions.Actor, so this always resolved to false regardless of min — a
// conditional stat func on a summon-owned skill silently never applied.
type levelGate struct{ min int }

func (g levelGate) Test(effector stat.Actor) bool {
	actor, ok := effector.(conditions.Actor)
	return ok && actor.Level() >= g.min
}

func TestSummonConditionalStatFuncGatesOnRealLevel(t *testing.T) {
	a := NewServitor(ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	// HealEffectiveness carries no default summon stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	a.AddStatFuncs([]effect.Mod{
		{Stat: stat.HealEffectiveness, Op: effect.OpAdd, Value: 25, Cond: levelGate{min: 10}},
	})
	a.AddStatFuncs([]effect.Mod{
		{Stat: stat.RechargeMPRate, Op: effect.OpAdd, Value: 25, Cond: levelGate{min: 100}},
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
