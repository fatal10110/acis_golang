package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestFusionEffectActionNeverEndsOnItsOwnTick(t *testing.T) {
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true: a fusion effect never ends via its own periodic tick")
	}
}

func TestFusionEffectIncreaseEffectGrowsLevelAndReapplies(t *testing.T) {
	target := &liveEffectTarget{list: newTestList(nil)}
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	var reappliedAt int
	e.IncreaseEffect(target.list, 5, func(level int) { reappliedAt = level })

	if e.Level != 4 {
		t.Fatalf("Level after IncreaseEffect = %d, want 4", e.Level)
	}
	if reappliedAt != 4 {
		t.Fatalf("reapply level = %d, want 4", reappliedAt)
	}
	if hasEffectInList(target.list, e) {
		t.Error("the prior instance must be removed before its replacement is applied")
	}
}

func TestFusionEffectIncreaseEffectAtMaxLevelIsANoop(t *testing.T) {
	target := &liveEffectTarget{list: newTestList(nil)}
	e, err := New(Skill{Level: 5}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	reapplied := false
	e.IncreaseEffect(target.list, 5, func(int) { reapplied = true })

	if e.Level != 5 {
		t.Fatalf("Level after IncreaseEffect at max = %d, want unchanged 5", e.Level)
	}
	if reapplied {
		t.Error("IncreaseEffect at max level must not reapply")
	}
	if !hasEffectInList(target.list, e) {
		t.Error("IncreaseEffect at max level must leave the instance in place")
	}
}

func TestFusionEffectDecreaseForceShrinksLevelAndReapplies(t *testing.T) {
	target := &liveEffectTarget{list: newTestList(nil)}
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	var reappliedAt int
	e.DecreaseForce(target.list, func(level int) { reappliedAt = level })

	if e.Level != 2 {
		t.Fatalf("Level after DecreaseForce = %d, want 2", e.Level)
	}
	if reappliedAt != 2 {
		t.Fatalf("reapply level = %d, want 2", reappliedAt)
	}
	if hasEffectInList(target.list, e) {
		t.Error("the prior instance must be removed before its replacement is applied")
	}
}

func TestFusionEffectDecreaseForceBelowOneRemovesWithoutReapply(t *testing.T) {
	target := &liveEffectTarget{list: newTestList(nil)}
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	reapplied := false
	e.DecreaseForce(target.list, func(int) { reapplied = true })

	if e.Level != 0 {
		t.Fatalf("Level after DecreaseForce below 1 = %d, want 0", e.Level)
	}
	if reapplied {
		t.Error("DecreaseForce dropping below level 1 must not reapply")
	}
	if hasEffectInList(target.list, e) {
		t.Error("DecreaseForce dropping below level 1 must remove the instance")
	}
}

func TestSeedEffectNeverEndsViaItsOwnTick(t *testing.T) {
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.ActionTime() {
		t.Fatal("ActionTime() = true, want false: a seed effect has no periodic tick behavior")
	}
}

func TestSeedEffectStartsAtSkillLevel(t *testing.T) {
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.Level != 1 {
		t.Fatalf("initial Level = %d, want 1 (matching EffectSeed's initial power)", e.Level)
	}
}

// TestSeedEffectPowerIgnoresSkillLevel guards EffectSeed.java:10's
// unconditional `_power = 1`: the initial charge must stay 1 even for a
// higher-level SEED skill, not track skill.getLevel() the way every other
// effect kind's Level does.
func TestSeedEffectPowerIgnoresSkillLevel(t *testing.T) {
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.Level != 1 {
		t.Fatalf("initial Level = %d, want 1 regardless of skill level 3 (EffectSeed._power = 1 is hardcoded)", e.Level)
	}
}

func TestSeedEffectIncreasePowerGrowsLevelInPlace(t *testing.T) {
	target := &liveEffectTarget{list: newTestList(nil)}
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	e.IncreasePower()

	if e.Level != 2 {
		t.Fatalf("Level after IncreasePower = %d, want 2", e.Level)
	}
	if !hasEffectInList(target.list, e) {
		t.Error("IncreasePower must grow the same instance in place, not replace it")
	}
}

// TestSeedRecastDoesNotExtendDeadline guards against reintroducing a
// reschedule-on-recast call. AbstractEffect.rescheduleEffect() ->
// startEffectTask()'s initialDelay = max((_period - getTime())*1000, 5) is
// derived from the effect's construction-time _periodStartTime, which
// rescheduleEffect() never mutates (AbstractEffect.java:264-270, 186-206,
// 138-141) — so a recast reproduces the same original deadline instead of
// granting a fresh full period. In this port that means growing a seed's
// power in place must leave its already-set nextAction/remaining alone.
func TestSeedRecastDoesNotExtendDeadline(t *testing.T) {
	target := &liveEffectTarget{list: newTestList(nil)}
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	t0 := time.Now()
	e.startSchedule(t0)
	wantDeadline := e.nextAction
	wantRemaining := e.remaining

	e.IncreasePower()

	if e.nextAction != wantDeadline {
		t.Fatalf("nextAction after recast = %v, want unchanged %v (reference pins the deadline)", e.nextAction, wantDeadline)
	}
	if e.remaining != wantRemaining {
		t.Fatalf("remaining after recast = %d, want unchanged %d", e.remaining, wantRemaining)
	}
}

// noBonusHealTarget implements only the minimum heal capability, to
// exercise the healStart/manaHealStart fallback defaults when the optional

func hasEffectInList(list *List, e *Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}
