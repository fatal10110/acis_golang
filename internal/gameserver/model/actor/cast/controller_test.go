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

func TestHitStopsCastWhenFinalCostsCannotBePaid(t *testing.T) {
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
		if ctrl.CastingNow() {
			t.Fatal("CastingNow() = true after final MP failure, want stopped")
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
		if ctrl.CastingNow() {
			t.Fatal("CastingNow() = true after final HP failure, want stopped")
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

type testTarget struct{}

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
}

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
