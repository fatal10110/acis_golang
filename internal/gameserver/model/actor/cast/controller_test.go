package cast

import (
	"errors"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestStartScalesTimingAndInstallsReuse(t *testing.T) {
	now := time.Unix(1000, 0)
	actor := &testActor{
		mp:             100,
		hp:             1000,
		mAtkSpd:        666,
		pAtkSpd:        333,
		magicReuseRate: 1.25,
		initialCost:    7,
		spiritshot:     true,
	}
	ctrl := NewController(actor)
	def := modelskill.Definition{
		ID:         10,
		Level:      2,
		Magic:      true,
		HitTime:    1500,
		CoolTime:   600,
		ReuseDelay: 12000,
	}

	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if plan.ReuseKey != 10*256+2 {
		t.Fatalf("ReuseKey = %d, want %d", plan.ReuseKey, 10*256+2)
	}
	if plan.HitTime != 525*time.Millisecond || plan.CoolTime != 210*time.Millisecond || plan.ReuseDelay != 7500*time.Millisecond {
		t.Fatalf("timing = hit %s cool %s reuse %s, want 525ms 210ms 7.5s", plan.HitTime, plan.CoolTime, plan.ReuseDelay)
	}
	if plan.LaunchDelay != 125*time.Millisecond || plan.HitDelay != 400*time.Millisecond || plan.FinalDelay != 210*time.Millisecond {
		t.Fatalf("phase delays = launch %s hit %s final %s, want 125ms 400ms 210ms", plan.LaunchDelay, plan.HitDelay, plan.FinalDelay)
	}
	if plan.InterruptAfter != 325*time.Millisecond || plan.GaugeDuration != 525*time.Millisecond {
		t.Fatalf("interrupt/gauge = %s/%s, want 325ms/525ms", plan.InterruptAfter, plan.GaugeDuration)
	}
	if actor.mp != 93 {
		t.Fatalf("MP after start = %d, want 93", actor.mp)
	}
	if len(actor.disabled) != 1 || actor.disabled[0].key != plan.ReuseKey || actor.disabled[0].delay != 7500*time.Millisecond {
		t.Fatalf("disabled cooldowns = %+v, want one 7.5s cooldown for reuse key", actor.disabled)
	}
	if len(actor.reuses) != 0 {
		t.Fatalf("stored reuse timestamps = %+v, want none below 30s", actor.reuses)
	}
}

// TestStartSkipsScalingForFusionSkills pins PlayerCast.doFusionCast
// (PlayerCast.java:75-76,52), which reads skill.getHitTime()/getCoolTime()/
// getReuseDelay() raw with no atkSpd or reuse-rate scaling. With the same
// atkSpd fixture as TestStartScalesTimingAndInstallsReuse (which would
// otherwise scale a 15000ms hitTime down to ~5250ms), a FUSION skill must
// keep the raw values.
func TestStartSkipsScalingForFusionSkills(t *testing.T) {
	now := time.Unix(1000, 0)
	actor := &testActor{
		mp:             100,
		hp:             1000,
		mAtkSpd:        666,
		pAtkSpd:        333,
		magicReuseRate: 1.25,
	}
	ctrl := NewController(actor)
	def := modelskill.Definition{
		ID:         426,
		Level:      1,
		Magic:      true,
		SkillType:  "FUSION",
		HitTime:    15000,
		ReuseDelay: 30000,
	}

	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if plan.HitTime != 15000*time.Millisecond {
		t.Fatalf("HitTime = %s, want raw 15000ms unscaled", plan.HitTime)
	}
	if plan.ReuseDelay != 30000*time.Millisecond {
		t.Fatalf("ReuseDelay = %s, want raw 30000ms unscaled", plan.ReuseDelay)
	}
	if plan.InterruptAfter != 14800*time.Millisecond {
		t.Fatalf("InterruptAfter = %s, want 14800ms (raw hitTime-200)", plan.InterruptAfter)
	}
	if plan.LaunchDelay != 14600*time.Millisecond || plan.GaugeDuration != 15000*time.Millisecond {
		t.Fatalf("LaunchDelay/GaugeDuration = %s/%s, want 14600ms/15000ms", plan.LaunchDelay, plan.GaugeDuration)
	}
}

func TestStartUsesSharedReuseAndStoresLongCooldowns(t *testing.T) {
	now := time.Unix(1000, 0)
	actor := &testActor{mp: 100, hp: 1000, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	def := modelskill.Definition{
		ID:          10,
		Level:       2,
		ReuseDelay:  40000,
		StaticReuse: true,
		SharedReuse: &modelskill.Ref{ID: 99, Level: 3},
	}

	plan, err := ctrl.Start(now, testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	wantKey := int32(99*256 + 3)
	if plan.ReuseKey != wantKey {
		t.Fatalf("ReuseKey = %d, want shared key %d", plan.ReuseKey, wantKey)
	}
	if len(actor.reuses) != 1 || actor.reuses[0].ref != (modelskill.Ref{ID: 10, Level: 2}) || actor.reuses[0].key != wantKey || actor.reuses[0].delay != 40*time.Second {
		t.Fatalf("stored reuses = %+v, want one 40s source-skill timestamp keyed by shared reuse", actor.reuses)
	}
	if len(actor.disabled) != 1 || actor.disabled[0].key != wantKey || actor.disabled[0].delay != 40*time.Second {
		t.Fatalf("disabled cooldowns = %+v, want one shared 40s cooldown", actor.disabled)
	}
}

func TestCanCastChecksCostsItemsReuseAndMute(t *testing.T) {
	def := modelskill.Definition{
		ID:               1,
		Level:            1,
		Magic:            true,
		MPConsume:        10,
		MPInitialConsume: 5,
		HPConsume:        20,
		ItemConsumeID:    57,
		ItemConsumeCount: 3,
	}

	actor := &testActor{mp: 14, hp: 21, items: map[int]int{57: 3}}
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrNotEnoughMP) {
		t.Fatalf("CanCast() error = %v, want ErrNotEnoughMP", err)
	}

	actor.mp = 15
	actor.hp = 20
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrNotEnoughHP) {
		t.Fatalf("CanCast() error = %v, want ErrNotEnoughHP", err)
	}

	actor.hp = 21
	actor.items[57] = 2
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrNotEnoughItems) {
		t.Fatalf("CanCast() error = %v, want ErrNotEnoughItems", err)
	}

	actor.items[57] = 3
	actor.magicMuted = true
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrMagicMuted) {
		t.Fatalf("CanCast() error = %v, want ErrMagicMuted", err)
	}

	actor.magicMuted = false
	actor.disabledKeys = map[int32]bool{1*256 + 1: true}
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrSkillDisabled) {
		t.Fatalf("CanCast() error = %v, want ErrSkillDisabled", err)
	}
}

func TestCanCastRejectsAllSkillsDisabled(t *testing.T) {
	def := modelskill.Definition{ID: 1, Level: 1}

	actor := &testActor{mp: 100, hp: 100, allDisabled: true}
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrAllSkillsDisabled) {
		t.Fatalf("CanCast() error = %v, want ErrAllSkillsDisabled", err)
	}

	actor.allDisabled = false
	if err := NewController(actor).CanCast(testTarget{}, def); err != nil {
		t.Fatalf("CanCast() error = %v, want nil once the lock clears", err)
	}
}

func TestCanCastRejectsSelfCubicSkillWhenCubicListFull(t *testing.T) {
	def := modelskill.Definition{
		ID: 1, Level: 1, SkillType: "SUMMON", IsCubic: true, Target: modelskill.TargetSelf,
	}

	actor := &testActor{mp: 100, hp: 100, cubicFull: true}
	if err := NewController(actor).CanCast(testTarget{}, def); !errors.Is(err, ErrCubicListFull) {
		t.Fatalf("CanCast() error = %v, want ErrCubicListFull", err)
	}

	actor.cubicFull = false
	if err := NewController(actor).CanCast(testTarget{}, def); err != nil {
		t.Fatalf("CanCast() error = %v, want nil once the list has room", err)
	}
}

func TestCanCastIgnoresCubicListFullForMassCubicTarget(t *testing.T) {
	def := modelskill.Definition{
		ID: 1, Level: 1, SkillType: "SUMMON", IsCubic: true, Target: modelskill.TargetParty,
	}

	actor := &testActor{mp: 100, hp: 100, cubicFull: true}
	if err := NewController(actor).CanCast(testTarget{}, def); err != nil {
		t.Fatalf("CanCast() error = %v, want nil: the full-list gate only applies to SELF-target cubic skills", err)
	}
}

func TestStartConsumesRequiredItems(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, items: map[int]int{57: 3}}
	ctrl := NewController(actor)
	def := modelskill.Definition{ID: 1, Level: 1, ItemConsumeID: 57, ItemConsumeCount: 3}

	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if actor.items[57] != 0 {
		t.Fatalf("item count after start = %d, want 0", actor.items[57])
	}
}

func TestHitConsumesFinalCostsAndAllowsExactHP(t *testing.T) {
	actor := &testActor{mp: 30, hp: 11, initialCost: 3, hitCost: 6}
	ctrl := NewController(actor)
	def := modelskill.Definition{ID: 1, Level: 1, HPConsume: 10}

	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	actor.hp = 10

	if err := ctrl.Hit(); err != nil {
		t.Fatalf("Hit() error: %v", err)
	}
	if actor.mp != 21 || actor.hp != 0 {
		t.Fatalf("resources after hit = mp %d hp %d, want 21/0", actor.mp, actor.hp)
	}
}

// TestHitAppliesForceSoulCharges pins CreatureCast.onMagicHitTimer's charge
// block (CreatureCast.java:276-282): a skill with NumCharges > 0 and
// MaxCharges > 0 increases the caster's charges at hit time; NumCharges > 0
// with MaxCharges == 0 decreases instead — both before Hooks.Hit would
// apply the skill's own effects.
func TestHitAppliesForceSoulCharges(t *testing.T) {
	t.Run("increase", func(t *testing.T) {
		actor := &chargingTestActor{testActor: testActor{mp: 30, hp: 30}}
		ctrl := NewController(actor)
		def := modelskill.Definition{ID: 461, Level: 1, NumCharges: 1, MaxCharges: 2}
		if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		if err := ctrl.Hit(); err != nil {
			t.Fatalf("Hit() error: %v", err)
		}
		if actor.increaseCalls != 1 || actor.increaseCount != 1 || actor.maxCount != 2 {
			t.Fatalf("IncreaseCharges calls=%d count=%d max=%d, want 1/1/2", actor.increaseCalls, actor.increaseCount, actor.maxCount)
		}
		if actor.decreaseCalls != 0 {
			t.Fatalf("DecreaseCharges calls = %d, want 0", actor.decreaseCalls)
		}
	})

	t.Run("decrease", func(t *testing.T) {
		actor := &chargingTestActor{testActor: testActor{mp: 30, hp: 30}}
		ctrl := NewController(actor)
		def := modelskill.Definition{ID: 462, Level: 1, NumCharges: 1}
		if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		if err := ctrl.Hit(); err != nil {
			t.Fatalf("Hit() error: %v", err)
		}
		if actor.decreaseCalls != 1 || actor.decreaseCount != 1 {
			t.Fatalf("DecreaseCharges calls=%d count=%d, want 1/1", actor.decreaseCalls, actor.decreaseCount)
		}
		if actor.increaseCalls != 0 {
			t.Fatalf("IncreaseCharges calls = %d, want 0", actor.increaseCalls)
		}
	})

	t.Run("non-player actor is a no-op", func(t *testing.T) {
		actor := &testActor{mp: 30, hp: 30}
		ctrl := NewController(actor)
		def := modelskill.Definition{ID: 463, Level: 1, NumCharges: 1, MaxCharges: 2}
		if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		if err := ctrl.Hit(); err != nil {
			t.Fatalf("Hit() error: %v", err)
		}
	})
}

// TestHitReportsUnpayableFinalCosts pins Hit's half of the contract only:
// it reports why the cast cannot be paid for and charges nothing, leaving
// the cast in flight for the abort funnel to cancel. The scheduled path
// that actually cancels it is TestUnaffordableHitReportsBeforeTheAbortFunnel.
func TestHitReportsUnpayableFinalCosts(t *testing.T) {
	t.Run("mp", func(t *testing.T) {
		actor := &testActor{mp: 30, hp: 100, hitCost: 20}
		ctrl := NewController(actor)
		if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, modelskill.Definition{ID: 1, Level: 1}); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		actor.mp = 10

		if err := ctrl.Hit(); !errors.Is(err, ErrNotEnoughMP) {
			t.Fatalf("Hit() error = %v, want ErrNotEnoughMP", err)
		}
		if actor.mp != 10 {
			t.Fatalf("MP = %d, want 10; an unaffordable hit must charge nothing", actor.mp)
		}
		if !ctrl.CastingNow() {
			t.Fatal("CastingNow() = false after final MP failure; Hit reports, the abort funnel stops")
		}
	})

	t.Run("hp", func(t *testing.T) {
		actor := &testActor{mp: 30, hp: 30}
		ctrl := NewController(actor)
		if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, modelskill.Definition{ID: 1, Level: 1, HPConsume: 20}); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		actor.hp = 10

		if err := ctrl.Hit(); !errors.Is(err, ErrNotEnoughHP) {
			t.Fatalf("Hit() error = %v, want ErrNotEnoughHP", err)
		}
		if actor.hp != 10 {
			t.Fatalf("HP = %d, want 10; an unaffordable hit must charge nothing", actor.hp)
		}
		if !ctrl.CastingNow() {
			t.Fatal("CastingNow() = false after final HP failure; Hit reports, the abort funnel stops")
		}
	})
}

func TestInterruptOnDamageHonorsWindowAndMagicOnlyRule(t *testing.T) {
	now := time.Unix(1000, 0)
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	magic := modelskill.Definition{ID: 1, Level: 1, Magic: true, HitTime: 1000}

	if _, err := ctrl.Start(now, testTarget{}, magic); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !ctrl.InterruptOnDamage(now.Add(100*time.Millisecond), DamageInterrupt{Damage: 100, MEN: 30, Roll: 0}) {
		t.Fatal("InterruptOnDamage() = false inside interrupt window with successful roll")
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after successful interrupt")
	}

	if _, err := ctrl.Start(now, testTarget{}, magic); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}
	if ctrl.InterruptOnDamage(now.Add(900*time.Millisecond), DamageInterrupt{Damage: 10000, MEN: 30, Roll: 0}) {
		t.Fatal("InterruptOnDamage() = true after interrupt window")
	}
	ctrl.Stop()

	physical := modelskill.Definition{ID: 2, Level: 1, Magic: false, HitTime: 1000}
	if _, err := ctrl.Start(now, testTarget{}, physical); err != nil {
		t.Fatalf("physical Start() error: %v", err)
	}
	if ctrl.InterruptOnDamage(now.Add(100*time.Millisecond), DamageInterrupt{Damage: 10000, MEN: 30, Roll: 0}) {
		t.Fatal("InterruptOnDamage() = true for physical skill")
	}
}

func TestStartSkillMasterySkipsReuseInstallation(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, mastery: true}
	ctrl := NewController(actor)
	def := modelskill.Definition{
		ID:            5,
		Level:         1,
		StaticHitTime: true,
		HitTime:       1000,
		StaticReuse:   true,
		ReuseDelay:    50000,
	}

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !plan.SkillMastery {
		t.Fatal("plan.SkillMastery = false, want true")
	}
	if plan.ReuseDelay != 50*time.Second {
		t.Fatalf("ReuseDelay = %s, want 50s (static, unscaled)", plan.ReuseDelay)
	}
	if len(actor.disabled) != 0 {
		t.Fatalf("disabled cooldowns = %+v, want none: skill mastery bypasses reuse installation even above both thresholds", actor.disabled)
	}
	if len(actor.reuses) != 0 {
		t.Fatalf("stored reuses = %+v, want none: skill mastery bypasses reuse installation even above both thresholds", actor.reuses)
	}
}

func TestStartRejectsSecondCastWhileActive(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor)
	def := modelskill.Definition{ID: 1, Level: 1}

	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); !errors.Is(err, ErrAlreadyCasting) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyCasting", err)
	}
}

func TestCanCastRejectsNilTarget(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	if err := NewController(actor).CanCast(nil, modelskill.Definition{ID: 1, Level: 1}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("CanCast(nil target) error = %v, want ErrInvalidTarget", err)
	}
}

func TestBuildPlanFloorsShortHitTimeToFiveHundred(t *testing.T) {
	// A physical skill with a configured hitTime of exactly 500 (the floor's
	// own threshold) whose attack speed scales it down under 500 gets
	// clamped back up, but only because the configured hitTime was already
	// >= 500; coolTime has no such floor.
	actor := &testActor{mp: 100, hp: 100, pAtkSpd: 1000}
	ctrl := NewController(actor)
	def := modelskill.Definition{ID: 1, Level: 1, Magic: false, HitTime: 500, CoolTime: 0, StaticReuse: true}

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	// Unfloored: int(500*333/1000) = 166.
	if plan.HitTime != 500*time.Millisecond {
		t.Fatalf("HitTime = %s, want 500ms (floored from a scaled 166ms)", plan.HitTime)
	}
	if plan.InterruptAfter != 300*time.Millisecond {
		t.Fatalf("InterruptAfter = %s, want 300ms (floored hitTime - 200ms)", plan.InterruptAfter)
	}
	if plan.LaunchDelay != 100*time.Millisecond || plan.HitDelay != 400*time.Millisecond || plan.GaugeDuration != 500*time.Millisecond {
		t.Fatalf("phase delays = launch %s hit %s gauge %s, want 100ms/400ms/500ms", plan.LaunchDelay, plan.HitDelay, plan.GaugeDuration)
	}
	if plan.FinalDelay != 0 {
		t.Fatalf("FinalDelay = %s, want 0: coolTime is 0 even though hitTime cleared the 410ms gauge threshold", plan.FinalDelay)
	}
}

func TestBuildPlanZeroesPhaseDelaysBelowGaugeThreshold(t *testing.T) {
	// A short static-hitTime skill (<=410ms) never crosses the gauge/launch
	// threshold, so every phase delay collapses to its zero value.
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor)
	def := modelskill.Definition{ID: 1, Level: 1, StaticHitTime: true, HitTime: 300, CoolTime: 50, StaticReuse: true}

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if plan.HitTime != 300*time.Millisecond || plan.CoolTime != 50*time.Millisecond {
		t.Fatalf("timing = hit %s cool %s, want 300ms/50ms (static, unscaled)", plan.HitTime, plan.CoolTime)
	}
	if plan.InterruptAfter != 100*time.Millisecond {
		t.Fatalf("InterruptAfter = %s, want 100ms (hitTime - 200ms, unconditional)", plan.InterruptAfter)
	}
	if plan.LaunchDelay != 0 || plan.HitDelay != 0 || plan.GaugeDuration != 0 || plan.FinalDelay != 0 {
		t.Fatalf("phase delays = launch %s hit %s gauge %s final %s, want all zero below the 410ms threshold",
			plan.LaunchDelay, plan.HitDelay, plan.GaugeDuration, plan.FinalDelay)
	}
}

func TestInterruptOnDamageImmuneNeverBreaks(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	now := time.Unix(1000, 0)
	if _, err := ctrl.Start(now, testTarget{}, modelskill.Definition{ID: 1, Level: 1, Magic: true, HitTime: 1000}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if ctrl.InterruptOnDamage(now.Add(100*time.Millisecond), DamageInterrupt{Damage: 1e9, MEN: 30, Roll: 0, Immune: true}) {
		t.Fatal("InterruptOnDamage() = true for an immune target, want false")
	}
	if !ctrl.CastingNow() {
		t.Fatal("CastingNow() = false after an immune-blocked hit, want still casting")
	}
}

func TestInterruptOnDamageFusionBypassesMagicAndRollRules(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	now := time.Unix(1000, 0)

	// Fusion breaks a cast unconditionally within the interrupt window, even
	// for a physical skill and even with a roll that would otherwise fail
	// the magic-only cast-break rate check.
	physical := modelskill.Definition{ID: 1, Level: 1, Magic: false, HitTime: 1000}
	if _, err := ctrl.Start(now, testTarget{}, physical); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !ctrl.InterruptOnDamage(now.Add(100*time.Millisecond), DamageInterrupt{Damage: 1, MEN: 99, Roll: 99, Fusion: true}) {
		t.Fatal("InterruptOnDamage() = false for a fusion break on a physical skill inside the window, want true")
	}

	// Outside the interrupt window, fusion still respects the abort window.
	magic := modelskill.Definition{ID: 2, Level: 1, Magic: true, HitTime: 1000}
	if _, err := ctrl.Start(now, testTarget{}, magic); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}
	if ctrl.InterruptOnDamage(now.Add(900*time.Millisecond), DamageInterrupt{Damage: 1, MEN: 0, Roll: 0, Fusion: true}) {
		t.Fatal("InterruptOnDamage() = true for a fusion break after the abort window, want false")
	}
}

// TestInterruptCastOnDamageBreaksActiveFusionChannel pins the wiring
// Formulas.calcCastBreak's fusion branch (Formulas.java:732-736) needs: any
// damage to a caster mid fusion-channel interrupts unconditionally, with no
// rate roll. InterruptCastOnDamage must derive DamageInterrupt.Fusion from
// live ScheduleFusion state instead of always passing false.
func TestInterruptCastOnDamageBreaksActiveFusionChannel(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	now := time.Now()

	def := modelskill.Definition{ID: 426, Level: 1, Magic: true, SkillType: "FUSION", HitTime: 15000}
	if _, err := ctrl.Start(now, testTarget{}, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !ctrl.ScheduleFusion(ctrl.plan, 0, nil, func() {}) {
		t.Fatal("ScheduleFusion() = false, want true")
	}

	// Roll 99 and MEN 99 would fail the ordinary magic cast-break rate
	// check, but a fusion channel bypasses it entirely.
	if !ctrl.InterruptCastOnDamage(1, 99, func(base float64) float64 { return base }, 99, false) {
		t.Fatal("InterruptCastOnDamage() = false while channeling fusion, want true")
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after a fusion-channel damage break, want false")
	}
}

// TestInterruptCastOnDamageBreaksFusionBeforeScheduleFusion pins that the
// fusion flag comes from the cast's own definition (c.current), not
// c.fusionEnd. PlayerCast.doFusionCast creates FusionSkill synchronously in
// the same call that flips isCastingNow=true (PlayerCast.java:79-82), so
// Formulas.calcCastBreak's unconditional fusion break applies from the
// moment the cast starts. In Go, ScheduleFusion (which used to gate the
// flag via fusionEnd) is only called by the network handler several
// statements after Start — deriving the flag from fusionEnd would leave
// exactly that gap unprotected. Skill 426/427 are non-magic (no isMagic in
// the datapack), so a false fusion reading here would additionally fall
// through InterruptOnDamage's magic-only gate and never break at all.
func TestInterruptCastOnDamageBreaksFusionBeforeScheduleFusion(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	now := time.Now()

	def := modelskill.Definition{ID: 426, Level: 1, Magic: false, SkillType: "FUSION", HitTime: 15000}
	if _, err := ctrl.Start(now, testTarget{}, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// ScheduleFusion has not run yet: c.fusionEnd is still nil.
	if !ctrl.InterruptCastOnDamage(1, 99, func(base float64) float64 { return base }, 99, false) {
		t.Fatal("InterruptCastOnDamage() = false before ScheduleFusion, want true")
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after a pre-ScheduleFusion damage break, want false")
	}
}

func TestCanAbort(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
	ctrl := NewController(actor)
	now := time.Unix(1000, 0)

	if ctrl.CanAbort(now) {
		t.Fatal("CanAbort() = true with no active cast, want false")
	}

	if _, err := ctrl.Start(now, testTarget{}, modelskill.Definition{ID: 1, Level: 1, Magic: true, HitTime: 1000}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !ctrl.CanAbort(now.Add(100 * time.Millisecond)) {
		t.Fatal("CanAbort() = false inside the interrupt window, want true")
	}
	if ctrl.CanAbort(now.Add(900 * time.Millisecond)) {
		t.Fatal("CanAbort() = true after the interrupt window, want false")
	}

	ctrl.Finish()
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after Finish(), want false")
	}
}

// TestInterruptOnDamageWindowBoundaryTable pins the exact interrupt-window
// edge at hitTime-200 (here 800ms into a 1000ms hitTime): a damage-break
// roll on either side of that instant must land on the side its timestamp
// implies, with the roll itself injected deterministically rather than
// relying on real randomness.
func TestInterruptOnDamageWindowBoundaryTable(t *testing.T) {
	now := time.Unix(1000, 0)
	def := modelskill.Definition{ID: 1, Level: 1, Magic: true, HitTime: 1000}

	tests := []struct {
		name   string
		offset time.Duration
		want   bool
	}{
		{name: "1ms inside the window", offset: 799 * time.Millisecond, want: true},
		{name: "exactly at the window edge", offset: 800 * time.Millisecond, want: false},
		{name: "1ms outside the window", offset: 801 * time.Millisecond, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &testActor{mp: 100, hp: 100, mAtkSpd: 333, pAtkSpd: 333, magicReuseRate: 1, physicalReuseRate: 1}
			ctrl := NewController(actor)
			if _, err := ctrl.Start(now, testTarget{}, def); err != nil {
				t.Fatalf("Start() error: %v", err)
			}

			// Roll 0 against a rate the CastBreakRate formula always clamps
			// to at least 1: a deterministic guaranteed-break roll, so the
			// only variable under test is the window boundary itself.
			got := ctrl.InterruptOnDamage(now.Add(tt.offset), DamageInterrupt{Damage: 1, MEN: 30, Roll: 0})
			if got != tt.want {
				t.Fatalf("InterruptOnDamage() at +%s = %v, want %v", tt.offset, got, tt.want)
			}
			if got == ctrl.CastingNow() {
				t.Fatalf("CastingNow() = %v after InterruptOnDamage() = %v, want the opposite", ctrl.CastingNow(), got)
			}
		})
	}
}

type testTarget struct{}

func (testTarget) ObjectID() int32         { return 1 }
func (testTarget) Position() (x, y, z int) { return 0, 0, 0 }

type testActor struct {
	mp, hp int

	mAtkSpd, pAtkSpd                  int
	magicReuseRate, physicalReuseRate float64
	initialCost, hitCost              int
	spiritshot, blessedSpiritshot     bool
	magicMuted, physicalMuted         bool
	mastery                           bool

	items        map[int]int
	disabledKeys map[int32]bool
	disabled     []testCooldown
	reuses       []testReuse

	cubicFull   bool
	allDisabled bool
}

func (a *testActor) CubicListFull() bool { return a.cubicFull }

func (a *testActor) AllSkillsDisabled() bool { return a.allDisabled }
func (a *testActor) EnableAllSkills()        { a.allDisabled = false }

type testCooldown struct {
	key   int32
	delay time.Duration
}

type testReuse struct {
	ref   modelskill.Ref
	key   int32
	delay time.Duration
}

func (a *testActor) AttackSpeed(magic bool) int {
	if magic {
		if a.mAtkSpd == 0 {
			return 333
		}
		return a.mAtkSpd
	}
	if a.pAtkSpd == 0 {
		return 333
	}
	return a.pAtkSpd
}

func (a *testActor) ReuseRate(magic bool) float64 {
	if magic {
		if a.magicReuseRate == 0 {
			return 1
		}
		return a.magicReuseRate
	}
	if a.physicalReuseRate == 0 {
		return 1
	}
	return a.physicalReuseRate
}

func (a *testActor) MP() int { return a.mp }
func (a *testActor) HP() int { return a.hp }

func (a *testActor) MPInitialCost(def modelskill.Definition) int {
	if a.initialCost != 0 {
		return a.initialCost
	}
	return def.MPInitialConsume
}

func (a *testActor) MPCost(def modelskill.Definition) int {
	if a.hitCost != 0 {
		return a.hitCost
	}
	return def.MPConsume
}

func (a *testActor) ReduceMP(n int) { a.mp -= n }
func (a *testActor) ReduceHP(n int) { a.hp -= n }

func (a *testActor) SkillDisabled(key int32) bool {
	return a.disabledKeys[key]
}

func (a *testActor) DisableSkill(key int32, delay time.Duration) {
	a.disabled = append(a.disabled, testCooldown{key: key, delay: delay})
}

func (a *testActor) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	a.reuses = append(a.reuses, testReuse{ref: ref, key: key, delay: delay})
}

func (a *testActor) MagicMuted() bool               { return a.magicMuted }
func (a *testActor) PhysicalMuted() bool            { return a.physicalMuted }
func (a *testActor) SpiritshotCharged() bool        { return a.spiritshot }
func (a *testActor) BlessedSpiritshotCharged() bool { return a.blessedSpiritshot }
func (a *testActor) SkillMastery(modelskill.Definition) bool {
	return a.mastery
}

func (a *testActor) ItemCount(itemID int) int {
	if a.items == nil {
		return 0
	}
	return a.items[itemID]
}

func (a *testActor) ConsumeItem(itemID, count int) bool {
	if a.items == nil || a.items[itemID] < count {
		return false
	}
	a.items[itemID] -= count
	return true
}

// chargingTestActor adds the chargeHolder surface on top of testActor, for
// pinning that Controller.Hit applies Force/Soul charges — and that a bare
// testActor (an NPC/summon stand-in, which never implements chargeHolder)
// silently skips them, matching CreatureCast.onMagicHitTimer's
// `_actor instanceof Player` gate.
type chargingTestActor struct {
	testActor
	charges                 int
	increaseCount, maxCount int
	decreaseCount           int
	increaseCalls           int
	decreaseCalls           int
}

func (a *chargingTestActor) IncreaseCharges(count, max int) bool {
	a.increaseCalls++
	a.increaseCount, a.maxCount = count, max
	return true
}

func (a *chargingTestActor) DecreaseCharges(count int) bool {
	a.decreaseCalls++
	a.decreaseCount = count
	return true
}
