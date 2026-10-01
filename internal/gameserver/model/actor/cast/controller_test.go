package cast

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestAbortObserverFiresOnlyForAnInFlightCast(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name       string
		end        func(*Controller)
		start      bool
		wantAborts int
	}{
		{
			name:       "stop in flight",
			start:      true,
			end:        func(c *Controller) { c.Stop() },
			wantAborts: 1,
		},
		{
			name:       "stop while idle",
			end:        func(c *Controller) { c.Stop() },
			wantAborts: 0,
		},
		{
			name:       "natural finish",
			start:      true,
			end:        func(c *Controller) { c.Finish() },
			wantAborts: 0,
		},
		{
			name:       "stop after natural finish",
			start:      true,
			end:        func(c *Controller) { c.Finish(); c.Stop() },
			wantAborts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, _, aborts := newAbortController()
			if tt.start {
				if _, err := ctrl.Start(now, testTarget{}, scalingDef); err != nil {
					t.Fatalf("Start() error: %v", err)
				}
			}

			tt.end(ctrl)

			if event.Count[event.CastAborted](aborts) != tt.wantAborts {
				t.Fatalf("abort observer fired %d times, want %d", event.Count[event.CastAborted](aborts), tt.wantAborts)
			}
			if ctrl.CastingNow() {
				t.Fatal("CastingNow() = true after the cast ended, want cleared")
			}
		})
	}
}

// TestFinishObserverReportsTheCastThatEnded pins that CastFinished carries
// the skill that just ended, not the zero value: the network
// layer's PlayableAI.onEvtFinishedCasting (PlayableAI.java:43-63) port
// gates attack resume on that skill's NextActionIsAttack, so a stale or
// zero def would silently disable or wrongly enable the resume for every
// cast.
func TestFinishObserverReportsTheCastThatEnded(t *testing.T) {
	ctrl, _, rec := newAbortController()
	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	ctrl.Finish()

	finished := event.Of[event.CastFinished](rec)
	if len(finished) != 1 || finished[0].Skill.ID != scalingDef.ID || finished[0].Skill.Level != scalingDef.Level {
		t.Fatalf("finish events = %+v, want one for %+v", finished, scalingDef)
	}
}

// TestFinishRequestsShotRechargeForTheSkillsShotKind pins the finalizer's
// shot recharge (CreatureCast.onMagicFinalizer): a natural finish asks for
// the soulshot or spiritshot recharge the skill spends, ahead of the attack
// stance and CastFinished; a skill that spends neither, an aborted cast and a
// FUSION channel (whose hit timer aborts before any recharge) ask for none.
func TestFinishRequestsShotRechargeForTheSkillsShotKind(t *testing.T) {
	now := time.Unix(1000, 0)
	pdam := modelskill.Definition{ID: 1, Level: 1, SkillType: "PDAM", Offensive: true, HitTime: 1000}
	magic := modelskill.Definition{ID: 3, Level: 1, SkillType: "MDAM", Magic: true, Offensive: true, HitTime: 1000}
	finish := func(_ *testing.T, c *Controller, _ Plan) { c.Finish() }
	stop := func(_ *testing.T, c *Controller, _ Plan) { c.Stop() }

	tests := []struct {
		name string
		def  modelskill.Definition
		end  func(*testing.T, *Controller, Plan)
		want *event.ShotsRechargeRequested
	}{
		{name: "pdam finish", def: pdam, end: finish, want: &event.ShotsRechargeRequested{Physical: true}},
		{
			name: "blow finish",
			def:  modelskill.Definition{ID: 2, Level: 1, SkillType: "BLOW", Offensive: true, HitTime: 1000},
			end:  finish,
			want: &event.ShotsRechargeRequested{Physical: true},
		},
		{name: "magic finish", def: magic, end: finish, want: &event.ShotsRechargeRequested{Magic: true}},
		{
			name: "non-magic heal finish",
			def:  modelskill.Definition{ID: 4, Level: 1, SkillType: "HEAL", HitTime: 1000},
			end:  finish,
		},
		{name: "pdam stopped mid-cast", def: pdam, end: stop},
		{name: "magic stopped mid-cast", def: magic, end: stop},
		{
			name: "fusion channel completes",
			def:  modelskill.Definition{ID: 426, Level: 1, Magic: true, SkillType: "FUSION", HitTime: 15000},
			end: func(t *testing.T, c *Controller, plan Plan) {
				clock := newCastClock()
				c.SetQueue(clock.q)
				ended := 0
				if !c.ScheduleFusion(plan, time.Second, func() bool { return true }, func() { ended++ }) {
					t.Fatal("ScheduleFusion() = false for an in-flight cast")
				}
				clock.advance(plan.LaunchDelay)
				if ended != 1 || c.CastingNow() {
					t.Fatalf("fusion channel after launch delay: end calls %d, casting %v; want 1 and false", ended, c.CastingNow())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, _, rec := newAbortController()
			plan, err := ctrl.Start(now, testTarget{}, tt.def)
			if err != nil {
				t.Fatalf("Start() error: %v", err)
			}
			ctrl.SetLaunchTargets(1)

			tt.end(t, ctrl, plan)

			recharges := event.Of[event.ShotsRechargeRequested](rec)
			if tt.want == nil {
				if len(recharges) != 0 {
					t.Fatalf("recharge events = %+v, want none", recharges)
				}
				return
			}
			if len(recharges) != 1 || recharges[0] != *tt.want {
				t.Fatalf("recharge events = %+v, want exactly %+v", recharges, *tt.want)
			}
			var order []string
			for _, e := range rec.Events() {
				switch e.(type) {
				case event.ShotsRechargeRequested:
					order = append(order, "recharge")
				case event.AttackStanceRequested:
					order = append(order, "stance")
				case event.CastFinished:
					order = append(order, "finished")
				}
			}
			if want := []string{"recharge", "stance", "finished"}; !slices.Equal(order, want) {
				t.Fatalf("finalizer event order = %v, want %v", order, want)
			}
		})
	}
}

// TestFinishObserverReportsTheCastsTarget pins that CastFinished carries the
// cast's final target on every way a cast ends: the nextActionAttack
// follow-up attacks that target after a natural finish and after an abort
// alike, since an aborted cast also notifies the AI that it finished.
func TestFinishObserverReportsTheCastsTarget(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name string
		end  func(*Controller)
	}{
		{name: "natural finish", end: func(c *Controller) { c.Finish() }},
		{name: "stop", end: func(c *Controller) { c.Stop() }},
		{name: "interrupt", end: func(c *Controller) {
			if !c.Interrupt(now) {
				t.Fatal("Interrupt() = false inside the interrupt window")
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, _, rec := newAbortController()
			target := &fakeCastCreature{id: 42}
			if _, err := ctrl.Start(now, target, scalingDef); err != nil {
				t.Fatalf("Start() error: %v", err)
			}

			tt.end(ctrl)

			finished := event.Of[event.CastFinished](rec)
			if len(finished) != 1 {
				t.Fatalf("finish events = %+v, want exactly one", finished)
			}
			if finished[0].Target != attackable.Combatant(target) {
				t.Fatalf("CastFinished.Target = %v, want the cast's target %v", finished[0].Target, target)
			}
		})
	}
}

func TestFinishObserverReportsEveryInFlightCastOnce(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name  string
		end   func(*Controller)
		start bool
		want  []bool
	}{
		{name: "abort", start: true, end: func(c *Controller) { c.Stop() }, want: []bool{true}},
		{name: "natural finish", start: true, end: func(c *Controller) { c.Finish() }, want: []bool{false}},
		{name: "idle stop", end: func(c *Controller) { c.Stop() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, _, rec := newAbortController()
			if tt.start {
				if _, err := ctrl.Start(now, testTarget{}, scalingDef); err != nil {
					t.Fatalf("Start() error: %v", err)
				}
			}

			tt.end(ctrl)
			var got []bool
			for _, e := range event.Of[event.CastFinished](rec) {
				got = append(got, e.Interrupted)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("finish observer = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopCancelsPendingPhaseTimers(t *testing.T) {
	clock := newCastClock()
	ctrl, _, aborts := newAbortController()
	ctrl.SetQueue(clock.q)

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	var fired []string
	ctrl.Schedule(plan, Hooks{
		Launch: func() bool { fired = append(fired, "launch"); return true },
		Hit:    func() { fired = append(fired, "hit") },
		Finish: func() { fired = append(fired, "finish") },
	})

	ctrl.Stop()
	clock.advance(plan.LaunchDelay)
	clock.advance(plan.HitDelay)
	clock.advance(plan.FinalDelay)

	if len(fired) != 0 {
		t.Fatalf("phase hooks ran after Stop: %v", fired)
	}
	if event.Count[event.CastAborted](aborts) != 1 {
		t.Fatalf("abort observer fired %d times, want 1", event.Count[event.CastAborted](aborts))
	}
}

func TestUnaffordableHitReportsBeforeTheAbortFunnel(t *testing.T) {
	clock := newCastClock()
	ctrl, actor, aborts := newAbortController()
	ctrl.SetQueue(clock.q)

	actor.hitCost = 20

	plan, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, scalingDef)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	// Drained mid-cast, exactly how a real caster loses the MP it had when
	// the cast started.
	actor.mp = 10

	var order []string
	ctrl.Schedule(plan, Hooks{
		Launch: func() bool { return true },
		Hit:    func() { order = append(order, "hit") },
		Failed: func(err error) {
			if !errors.Is(err, ErrNotEnoughMP) {
				t.Fatalf("Failed hook error = %v, want ErrNotEnoughMP", err)
			}
			if event.Count[event.CastAborted](aborts) != 0 {
				t.Fatal("abort observer fired before the failure was reported")
			}
			order = append(order, "failed")
		},
	})

	clock.advance(plan.LaunchDelay)
	clock.advance(plan.HitDelay)

	if len(order) != 1 || order[0] != "failed" {
		t.Fatalf("hook order = %v, want only the failure hook", order)
	}
	if event.Count[event.CastAborted](aborts) != 1 {
		t.Fatalf("abort observer fired %d times, want 1", event.Count[event.CastAborted](aborts))
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after an unaffordable hit, want cleared")
	}
	if actor.mp != 10 {
		t.Fatalf("MP = %d, want 10; the unaffordable hit must charge nothing", actor.mp)
	}
}

// reentrantCostActor calls back into its own controller from every cost it
// pays: item and MP costs read the cast state, and the HP cost stops the cast
// the way a caster killed by its own HP cost does.
type reentrantCostActor struct {
	*testActor
	ctrl           *Controller
	consumeFail    bool
	stopOnConsume  bool
	stopOnReduceMP bool
}

func (a *reentrantCostActor) ConsumeItem(itemID, count int) bool {
	_ = a.ctrl.CastingNow()
	if a.stopOnConsume {
		a.ctrl.Stop()
	}
	return !a.consumeFail && a.testActor.ConsumeItem(itemID, count)
}

func (a *reentrantCostActor) ReduceMP(n int) {
	_ = a.ctrl.CastingNow()
	a.testActor.ReduceMP(n)
	if a.stopOnReduceMP {
		a.ctrl.Stop()
	}
}

func (a *reentrantCostActor) ReduceHP(n int) {
	a.testActor.ReduceHP(n)
	a.ctrl.Stop()
}

func newReentrantCostController() (*Controller, *reentrantCostActor, modelskill.Definition) {
	actor := &reentrantCostActor{testActor: scalingActor()}
	actor.items = map[int]int{57: 5}
	ctrl := NewController(actor, nil)
	actor.ctrl = ctrl
	def := scalingDef
	def.ItemConsumeID, def.ItemConsumeCount = 57, 1
	def.MPConsume = 5
	def.HPConsume = 3
	return ctrl, actor, def
}

func TestCastCostsMayCallBackIntoTheController(t *testing.T) {
	ctrl, actor, def := newReentrantCostController()
	rec := &event.Recorder{}
	ctrl.sink = rec

	done := make(chan error, 1)
	go func() {
		if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
			done <- err
			return
		}
		done <- ctrl.Hit()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start/Hit error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cast controller deadlocked on a cost that calls back into it")
	}

	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true, want the HP cost's Stop to have ended the cast")
	}
	if event.Count[event.CastAborted](rec) != 1 {
		t.Fatalf("abort observer fired %d times, want 1", event.Count[event.CastAborted](rec))
	}
	if actor.items[57] != 4 || actor.mp != 100-7-5 || actor.hp != 1000-3 {
		t.Fatalf("items=%d mp=%d hp=%d, want 4/88/997", actor.items[57], actor.mp, actor.hp)
	}
}

// TestStartRejectsACastItsOwnCostEnded pins that Start reports a failure for a
// cast that one of its own costs cancelled through Stop. Reporting success
// there would let the caller announce a cast the abort funnel already
// cancelled, leaving the client playing an animation nothing ends.
func TestStartRejectsACastItsOwnCostEnded(t *testing.T) {
	ctrl, actor, def := newReentrantCostController()
	actor.stopOnReduceMP = true
	rec := &event.Recorder{}
	ctrl.sink = rec

	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); !errors.Is(err, ErrNotCasting) {
		t.Fatalf("Start() error = %v, want ErrNotCasting", err)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true, want the cost's Stop to have ended the cast")
	}
	if event.Count[event.CastAborted](rec) != 1 {
		t.Fatalf("abort observer fired %d times, want 1", event.Count[event.CastAborted](rec))
	}
	// The costs charged before the cast ended stay charged, matching a
	// reference that pays reuse and the initial MP before it claims the cast.
	if actor.items[57] != 4 || actor.mp != 100-7 || len(actor.disabled) != 1 {
		t.Fatalf("items=%d mp=%d disabled=%v, want 4/93/one cooldown", actor.items[57], actor.mp, actor.disabled)
	}
}

// TestStartItemFailureAfterTheCastEnded covers the rollback path when the cast
// was already ended while ConsumeItem ran: the claim is gone, so Start must
// leave it alone instead of clearing whatever cast came after, and still
// report the item failure.
func TestStartItemFailureAfterTheCastEnded(t *testing.T) {
	ctrl, actor, def := newReentrantCostController()
	actor.consumeFail, actor.stopOnConsume = true, true
	rec := &event.Recorder{}
	ctrl.sink = rec

	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); !errors.Is(err, ErrNotEnoughItems) {
		t.Fatalf("Start() error = %v, want ErrNotEnoughItems", err)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after a rejected Start, want cleared")
	}
	if event.Count[event.CastAborted](rec) != 1 {
		t.Fatalf("abort observer fired %d times, want 1 (the cost's own Stop)", event.Count[event.CastAborted](rec))
	}
	if actor.mp != 100 || len(actor.disabled) != 0 || len(actor.reuses) != 0 {
		t.Fatalf("mp=%d disabled=%v reuses=%v, want nothing charged past the item failure", actor.mp, actor.disabled, actor.reuses)
	}

	actor.consumeFail, actor.stopOnConsume = false, false
	if _, err := ctrl.Start(time.Unix(1001, 0), testTarget{}, def); err != nil {
		t.Fatalf("Start() after the rejected Start error: %v", err)
	}
}

func TestStartThatCannotConsumeItsItemPaysNothing(t *testing.T) {
	ctrl, actor, def := newReentrantCostController()
	actor.consumeFail = true

	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); !errors.Is(err, ErrNotEnoughItems) {
		t.Fatalf("Start() error = %v, want ErrNotEnoughItems", err)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after a rejected Start, want cleared")
	}
	if actor.mp != 100 || len(actor.disabled) != 0 || len(actor.reuses) != 0 {
		t.Fatalf("mp=%d disabled=%v reuses=%v, want a rejected cast to pay nothing", actor.mp, actor.disabled, actor.reuses)
	}

	actor.consumeFail = false
	if _, err := ctrl.Start(time.Unix(1001, 0), testTarget{}, def); err != nil {
		t.Fatalf("Start() after a rejected Start error: %v", err)
	}
}

// TestStartCarriedConsumesCarrierBeforeCosts pins the item-carried cast
// order: the carrier goes once the cast is claimed and before the skill's
// own item, reuse and initial MP; a carrier that cannot be consumed releases
// the claim, charges nothing and is reported as is.
func TestStartCarriedConsumesCarrierBeforeCosts(t *testing.T) {
	errCarrier := errors.New("carrier gone")
	t.Run("consumed", func(t *testing.T) {
		ctrl, actor, def := newReentrantCostController()
		var seen struct {
			casting   bool
			items, mp int
			cooldowns int
		}
		plan, err := ctrl.StartCarried(time.Unix(1000, 0), testTarget{}, def, func() error {
			seen.casting, seen.items, seen.mp, seen.cooldowns = ctrl.CastingNow(), actor.items[57], actor.mp, len(actor.disabled)
			return nil
		})
		if err != nil {
			t.Fatalf("StartCarried() error: %v", err)
		}
		if plan.ItemCharge != ItemChargePaid {
			t.Fatalf("ItemCharge = %d, want ItemChargePaid", plan.ItemCharge)
		}
		if !seen.casting || seen.items != 5 || seen.mp != 100 || seen.cooldowns != 0 {
			t.Fatalf("at carrier consume: casting=%v items=%d mp=%d cooldowns=%d, want claimed with nothing charged (true/5/100/0)", seen.casting, seen.items, seen.mp, seen.cooldowns)
		}
		if actor.items[57] != 4 || actor.mp != 100-7 || len(actor.disabled) != 1 {
			t.Fatalf("after start: items=%d mp=%d disabled=%v, want 4/93/one cooldown", actor.items[57], actor.mp, actor.disabled)
		}
	})
	t.Run("own item short", func(t *testing.T) {
		// The carrier is paid, so the skill's own item failing to go does
		// not refuse the cast: it runs, charges its other costs, and reports
		// the item short.
		ctrl, actor, def := newReentrantCostController()
		actor.consumeFail = true
		plan, err := ctrl.StartCarried(time.Unix(1000, 0), testTarget{}, def, func() error { return nil })
		if err != nil {
			t.Fatalf("StartCarried() error = %v, want the cast started", err)
		}
		if plan.ItemCharge != ItemChargeShort {
			t.Fatalf("ItemCharge = %d, want ItemChargeShort", plan.ItemCharge)
		}
		if !ctrl.CastingNow() {
			t.Fatal("CastingNow() = false, want the carried cast in flight")
		}
		if actor.items[57] != 5 || actor.mp != 100-7 || len(actor.disabled) != 1 {
			t.Fatalf("items=%d mp=%d disabled=%v, want 5/93/one cooldown", actor.items[57], actor.mp, actor.disabled)
		}
	})
	t.Run("carrier missing", func(t *testing.T) {
		ctrl, actor, def := newReentrantCostController()
		rec := &event.Recorder{}
		ctrl.sink = rec
		if _, err := ctrl.StartCarried(time.Unix(1000, 0), testTarget{}, def, func() error { return errCarrier }); !errors.Is(err, errCarrier) {
			t.Fatalf("StartCarried() error = %v, want the carrier's error", err)
		}
		if ctrl.CastingNow() {
			t.Fatal("CastingNow() = true after a lost carrier, want the claim released")
		}
		if len(rec.Events()) != 0 {
			t.Fatalf("events = %v, want none for a cast never announced", rec.Events())
		}
		if actor.items[57] != 5 || actor.mp != 100 || len(actor.disabled) != 0 || len(actor.reuses) != 0 {
			t.Fatalf("items=%d mp=%d disabled=%v reuses=%v, want nothing charged", actor.items[57], actor.mp, actor.disabled, actor.reuses)
		}
		if _, err := ctrl.Start(time.Unix(1001, 0), testTarget{}, def); err != nil {
			t.Fatalf("Start() after a lost carrier error: %v", err)
		}
	})
}

func TestPlayerActorExitSignetGroundDropsOnlyTheSignetEffect(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	ch.Live = live

	lasting := modelskill.EffectTemplate{Time: 60}
	signet := &effect.Effect{Skill: effect.Skill{ID: 7}, Template: lasting, Type: effect.TypeSignetGround}
	ch.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 8}, Template: lasting, Type: effect.TypeBuff})
	ch.EffectList().Add(signet)

	PlayerActor{Character: ch}.ExitSignetGround()

	remaining := ch.EffectList().All()
	if len(remaining) != 1 || remaining[0].Type != effect.TypeBuff {
		t.Fatalf("effects after ExitSignetGround = %+v, want only the buff left", remaining)
	}
}

func TestStopRunsOwnerCleanupEvenWhenIdle(t *testing.T) {
	tests := []struct {
		name        string
		allDisabled bool
		wantEnables int
	}{
		{name: "skills already usable", wantEnables: 0},
		{name: "all skills disabled", allDisabled: true, wantEnables: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, actor, _ := newAbortController()
			actor.allDisabled = tt.allDisabled

			ctrl.Stop()

			if actor.signetExits != 1 {
				t.Fatalf("ExitSignetGround called %d times, want 1", actor.signetExits)
			}
			if actor.enableCalls != tt.wantEnables {
				t.Fatalf("EnableAllSkills called %d times, want %d", actor.enableCalls, tt.wantEnables)
			}
		})
	}
}

func (reentrantCostActor) DecreaseCharges(int) bool { return false }

func (reentrantCostActor) ExitSignetGround() {}

func (reentrantCostActor) GroundTargetUnset() bool { return false }

func (reentrantCostActor) IncreaseCharges(int, int) bool { return false }

// TestCanCastWeaponDependency pins L2Skill.getWeaponDependancy as CanCast
// runs it: a skill naming weapon or shield types casts only when the
// caster's held weapon/shield mask shares a bit with them, an unknown name
// restricts nothing, and a mute refusal answers first.
func TestCanCastWeaponDependency(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		allowed string
		held    int32
		muted   bool
		want    error
	}{
		{"unrestricted", "", 0, false, nil},
		{"bow skill without a bow", "BOW", item.WeaponFist.Mask(), false, ErrWeaponNotAllowed},
		{"bow skill with a bow", "BOW", item.WeaponBow.Mask(), false, nil},
		{"one of several types", "SWORD,BLUNT,BIGBLUNT,BIGSWORD", item.WeaponBigBlunt.Mask(), false, nil},
		{"shield skill with a sword and shield", "SHIELD", item.WeaponSword.Mask() | item.ArmorShield.Mask(), false, nil},
		{"shield skill with a sword alone", "SHIELD", item.WeaponSword.Mask(), false, ErrWeaponNotAllowed},
		{"holding nothing", "DAGGER", 0, false, ErrWeaponNotAllowed},
		{"unknown type name", "LANCE", 0, false, nil},
		{"mute answers first", "BOW", 0, true, ErrPhysicalMuted},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actor := &testActor{mp: 100, hp: 100, held: tt.held, physicalMuted: tt.muted}
			ctrl := NewController(actor, nil)
			def := modelskill.Definition{ID: 56, Level: 1, Activation: modelskill.ActivationActive, WeaponsAllowed: tt.allowed}
			if err := ctrl.CanCast(testTarget{}, def); !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Fatalf("CanCast = %v, want %v", err, tt.want)
			}
		})
	}
}
