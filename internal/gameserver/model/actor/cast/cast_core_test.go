package cast

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from abort_test.go ----
// abortActor adds the two optional owner surfaces the abort funnel consults
// to the shared cast test actor.
type abortActor struct {
	*testActor
	allDisabled bool
	enableCalls int
	signetExits int
}

func (a *abortActor) AllSkillsDisabled() bool { return a.allDisabled }

func (a *abortActor) EnableAllSkills() {
	a.enableCalls++
	a.allDisabled = false
}

func (a *abortActor) ExitSignetGround() { a.signetExits++ }

func newAbortController() (*Controller, *abortActor, *int) {
	actor := &abortActor{testActor: scalingActor()}
	ctrl := NewController(actor)
	aborts := new(int)
	ctrl.SetOnAbort(func(bool) { *aborts++ })
	return ctrl, actor, aborts
}

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

			if *aborts != tt.wantAborts {
				t.Fatalf("abort observer fired %d times, want %d", *aborts, tt.wantAborts)
			}
			if ctrl.CastingNow() {
				t.Fatal("CastingNow() = true after the cast ended, want cleared")
			}
		})
	}
}

// TestFinishObserverReportsTheCastThatEnded pins that SetOnFinish's def and
// target are the skill that just ended, not the zero value: the network
// layer's PlayableAI.onEvtFinishedCasting (PlayableAI.java:43-63) port
// gates attack resume on that skill's NextActionIsAttack, so a stale or
// zero def would silently disable or wrongly enable the resume for every
// cast.
func TestFinishObserverReportsTheCastThatEnded(t *testing.T) {
	ctrl, _, _ := newAbortController()
	target := testTarget{}
	var gotDef modelskill.Definition
	var gotTarget Target
	ctrl.SetOnFinish(func(_ bool, def modelskill.Definition, tgt Target) {
		gotDef, gotTarget = def, tgt
	})
	if _, err := ctrl.Start(time.Unix(1000, 0), target, scalingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	ctrl.Finish()

	if gotDef.ID != scalingDef.ID || gotDef.Level != scalingDef.Level {
		t.Fatalf("finish observer def = %+v, want %+v", gotDef, scalingDef)
	}
	if gotTarget != target {
		t.Fatalf("finish observer target = %v, want %v", gotTarget, target)
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
			ctrl, _, _ := newAbortController()
			var got []bool
			ctrl.SetOnFinish(func(interrupted bool, _ modelskill.Definition, _ Target) { got = append(got, interrupted) })
			if tt.start {
				if _, err := ctrl.Start(now, testTarget{}, scalingDef); err != nil {
					t.Fatalf("Start() error: %v", err)
				}
			}

			tt.end(ctrl)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("finish observer = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopCancelsPendingPhaseTimers(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl, _, aborts := newAbortController()
	ctrl.afterFunc = clock.AfterFunc

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
	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)
	clock.fire(plan.FinalDelay)

	if len(fired) != 0 {
		t.Fatalf("phase hooks ran after Stop: %v", fired)
	}
	if *aborts != 1 {
		t.Fatalf("abort observer fired %d times, want 1", *aborts)
	}
}

func TestUnaffordableHitReportsBeforeTheAbortFunnel(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl, actor, aborts := newAbortController()
	ctrl.afterFunc = clock.AfterFunc

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
			if *aborts != 0 {
				t.Fatal("abort observer fired before the failure was reported")
			}
			order = append(order, "failed")
		},
	})

	clock.fire(plan.LaunchDelay)
	clock.fire(plan.HitDelay)

	if len(order) != 1 || order[0] != "failed" {
		t.Fatalf("hook order = %v, want only the failure hook", order)
	}
	if *aborts != 1 {
		t.Fatalf("abort observer fired %d times, want 1", *aborts)
	}
	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after an unaffordable hit, want cleared")
	}
	if actor.mp != 10 {
		t.Fatalf("MP = %d, want 10; the unaffordable hit must charge nothing", actor.mp)
	}
}

func TestPlayerActorExitSignetGroundDropsOnlyTheSignetEffect(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
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

// ---- from ai_test.go ----
func TestAIControllerDisabledReflectsCastingNow(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor)
	ai := &AIController{Controller: ctrl, Definitions: fakeDefinitions{}}

	if ai.Disabled() {
		t.Fatal("Disabled() = true before any cast started")
	}

	def := modelskill.Definition{ID: 1, Level: 1, StaticHitTime: true, HitTime: 1000, StaticReuse: true}
	if _, err := ctrl.Start(time.Unix(1000, 0), testTarget{}, def); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if !ai.Disabled() {
		t.Fatal("Disabled() = false while mid-cast, want true")
	}
}

func TestAIControllerDisabledReflectsAllSkillsDisabled(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	ai := &AIController{Controller: NewController(actor), Definitions: fakeDefinitions{}}

	if ai.Disabled() {
		t.Fatal("Disabled() = true before the lock is set")
	}

	actor.allDisabled = true
	if !ai.Disabled() {
		t.Fatal("Disabled() = false with AllSkillsDisabled true, want true")
	}

	actor.allDisabled = false
	if ai.Disabled() {
		t.Fatal("Disabled() = true after the lock clears")
	}
}

func TestAIControllerRangeAndStopsMovementReadDefinition(t *testing.T) {
	ref := modelskill.Ref{ID: 5, Level: 1}
	ai := &AIController{
		Definitions: fakeDefinitions{ref: modelskill.Definition{CastRange: 600, HitTime: 1200}},
	}

	if got := ai.Range(ref); got != 600 {
		t.Fatalf("Range() = %d, want 600", got)
	}
	if !ai.StopsMovement(ref) {
		t.Fatal("StopsMovement() = false for a 1200ms hit time, want true")
	}

	shortRef := modelskill.Ref{ID: 6, Level: 1}
	ai.Definitions = fakeDefinitions{shortRef: modelskill.Definition{HitTime: 40}}
	if ai.StopsMovement(shortRef) {
		t.Fatal("StopsMovement() = true for a 40ms hit time, want false")
	}

	if got := ai.Range(modelskill.Ref{ID: 999}); got != 0 {
		t.Fatalf("Range() for an unknown ref = %d, want 0", got)
	}
}

func TestAIControllerCanAttemptReflectsCooldown(t *testing.T) {
	ref := modelskill.Ref{ID: 5, Level: 1}
	def := modelskill.Definition{ID: 5, Level: 1}
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor)
	ai := &AIController{Controller: ctrl, Definitions: fakeDefinitions{ref: def}}
	target := &fakeCastCreature{id: 2}

	if !ai.CanAttempt(target, ref) {
		t.Fatal("CanAttempt() = false with no cooldown installed")
	}

	actor.disabledKeys = map[int32]bool{ReuseKey(def): true}
	if ai.CanAttempt(target, ref) {
		t.Fatal("CanAttempt() = true while the reuse key is disabled")
	}
}

func TestAIControllerCanCastReflectsControllerGates(t *testing.T) {
	ref := modelskill.Ref{ID: 5, Level: 1}
	def := modelskill.Definition{ID: 5, Level: 1, MPConsume: 10}
	actor := &testActor{mp: 5, hp: 100}
	ctrl := NewController(actor)
	ai := &AIController{Controller: ctrl, Definitions: fakeDefinitions{ref: def}}
	target := &fakeCastCreature{id: 2}

	if ai.CanCast(target, ref) {
		t.Fatal("CanCast() = true without enough MP")
	}

	actor.mp = 10
	if !ai.CanCast(target, ref) {
		t.Fatal("CanCast() = false with enough MP and no other blockers")
	}
}

func TestAIControllerMeetsHPMPDisabledIgnoresReuse(t *testing.T) {
	ref := modelskill.Ref{ID: 5, Level: 1}
	def := modelskill.Definition{ID: 5, Level: 1, MPConsume: 10}
	actor := &testActor{mp: 10, hp: 100, disabledKeys: map[int32]bool{ReuseKey(def): true}}
	ctrl := NewController(actor)
	ai := &AIController{Controller: ctrl, Definitions: fakeDefinitions{ref: def}}
	target := &fakeCastCreature{id: 2}

	if !ai.MeetsHPMPDisabled(target, ref) {
		t.Fatal("MeetsHPMPDisabled() = false on reuse cooldown, want HP/MP/mute only")
	}
	if ai.CanCast(target, ref) {
		t.Fatal("CanCast() = true on reuse cooldown")
	}

	actor.mp = 5
	if ai.MeetsHPMPDisabled(target, ref) {
		t.Fatal("MeetsHPMPDisabled() = true without enough MP")
	}

	actor.mp = 10
	actor.magicMuted = true
	def.Magic = true
	ai.Definitions = fakeDefinitions{ref: def}
	if ai.MeetsHPMPDisabled(target, ref) {
		t.Fatal("MeetsHPMPDisabled() = true while magic muted")
	}
}

// TestAIControllerCastStartsSchedulesAndAppliesEffectsOnHit exercises
// AIController.Cast end to end: it must start and schedule the cast on
// Controller, then — only once the scheduled Hit phase runs — resolve and
// apply the skill's effects through the exact same ApplyEffects/
// EffectHandlers plumbing the live player cast pipeline drives.
func TestAIControllerCastStartsSchedulesAndAppliesEffectsOnHit(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{}
	caster := &fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}
	target := &fakeCastCreature{id: 2, category: skilltarget.CategoryAttackable}

	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     newEffectHandlers(effectsKnown{}, "DUMMYCAST", rec),
		Caster:      caster,
	}

	ai.Cast(target, ref)

	if !ctrl.CastingNow() {
		t.Fatal("CastingNow() = false right after Cast(), want mid-cast")
	}
	if len(rec.calls) != 0 {
		t.Fatal("skill handler ran before the Hit phase")
	}

	// scalingActor/scalingDef (schedule_test.go) is the same
	// oracle-verified fixture as TestStartScalesTimingAndInstallsReuse:
	// LaunchDelay 125ms, HitDelay 400ms.
	clock.fire(125 * time.Millisecond)
	clock.fire(400 * time.Millisecond)

	if len(rec.calls) != 1 {
		t.Fatalf("handler calls after Hit phase = %d, want 1", len(rec.calls))
	}
	if len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(target) {
		t.Fatalf("handler call targets = %v, want [target]", rec.calls[0].Targets)
	}
}

// TestAIControllerCastBroadcastsSkillUseAtStartAndLaunchedOnLaunch verifies
// the observer packet sequence CreatureCast.java wires: MagicSkillUse
// broadcasts the instant the cast starts, with the computed
// hitTime/reuseDelay (CreatureCast.java:148, inside doCast, before the
// launch timer at :165 is even scheduled), and MagicSkillLaunched
// broadcasts at the Launch phase — hitTime-400ms — not at Hit
// (CreatureCast.java:165,234). PlayerCast.doCast chains into this same
// CreatureCast path via super.doCast, so the live-player packet order in
// network/magic_skill.go (MagicSkillUse at cast start, MagicSkillLaunched
// in the Launch hook) is the same sequence this asserts for AI casters.
func TestAIControllerCastBroadcastsSkillUseAtStartAndLaunchedOnLaunch(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{}
	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	target := &fakeCastCreature{id: 2, x: 10, y: 20, z: 30, category: skilltarget.CategoryAttackable}

	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     newEffectHandlers(effectsKnown{}, "DUMMYCAST", rec),
		Caster:      caster,
	}

	ai.Cast(target, ref)

	if len(caster.skillUseCalls) != 1 {
		t.Fatalf("BroadcastSkillUse calls at cast start = %d, want 1", len(caster.skillUseCalls))
	}
	use := caster.skillUseCalls[0]
	if use.targetID != 2 || use.targetX != 10 || use.targetY != 20 || use.targetZ != 30 {
		t.Fatalf("BroadcastSkillUse target = %+v, want id 2 at (10,20,30)", use)
	}
	if use.skillID != int32(def.ID) || use.level != int32(def.Level) {
		t.Fatalf("BroadcastSkillUse skill = (%d,%d), want (%d,%d)", use.skillID, use.level, def.ID, def.Level)
	}
	if len(caster.skillLaunchedCalls) != 0 {
		t.Fatal("BroadcastSkillLaunched called before the Launch phase")
	}

	clock.fire(125 * time.Millisecond) // Launch

	if len(caster.skillUseCalls) != 1 {
		t.Fatalf("BroadcastSkillUse calls after Launch = %d, want still 1 (no re-broadcast)", len(caster.skillUseCalls))
	}
	if len(caster.skillLaunchedCalls) != 1 {
		t.Fatalf("BroadcastSkillLaunched calls after Launch = %d, want 1", len(caster.skillLaunchedCalls))
	}
	launched := caster.skillLaunchedCalls[0]
	if launched.skillID != int32(def.ID) || launched.level != int32(def.Level) {
		t.Fatalf("BroadcastSkillLaunched skill = (%d,%d), want (%d,%d)", launched.skillID, launched.level, def.ID, def.Level)
	}
	if len(launched.targetIDs) != 1 || launched.targetIDs[0] != 2 {
		t.Fatalf("BroadcastSkillLaunched targetIDs = %v, want [2]", launched.targetIDs)
	}

	clock.fire(400 * time.Millisecond) // Hit

	if len(caster.skillLaunchedCalls) != 1 {
		t.Fatalf("BroadcastSkillLaunched calls after Hit = %d, want still 1 (no re-broadcast)", len(caster.skillLaunchedCalls))
	}
}

// TestAIControllerCastBroadcastsSkillLaunchedWithFullTargetList verifies
// MagicSkillLaunched carries every affected target from the launch-resolved
// target list, not just the AI's preselected target — matching
// CreatureCast.java:232-234's `_targets = _skill.getTargetList(_actor,
// _target)` recompute and `broadcastPacket(new MagicSkillLaunched(_actor,
// _skill, _targets))` broadcast of the full array.
func TestAIControllerCastBroadcastsSkillLaunchedWithFullTargetList(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetArea
	def.Offensive = true
	def.Radius = 900
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{}
	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	selected := &fakeCastCreature{id: 2, x: 10, category: skilltarget.CategoryPlayable}
	bystander := &fakeCastCreature{id: 3, x: 20, category: skilltarget.CategoryPlayable}

	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     newEffectHandlers(effectsKnown{caster, selected, bystander}, "DUMMYCAST", rec),
		Caster:      caster,
	}

	ai.Cast(selected, ref)
	clock.fire(125 * time.Millisecond) // Launch

	if len(caster.skillLaunchedCalls) != 1 {
		t.Fatalf("BroadcastSkillLaunched calls after Launch = %d, want 1", len(caster.skillLaunchedCalls))
	}
	ids := caster.skillLaunchedCalls[0].targetIDs
	if len(ids) != 2 {
		t.Fatalf("BroadcastSkillLaunched targetIDs = %v, want 2 ids (selected + bystander)", ids)
	}
	seen := map[int32]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[2] || !seen[3] {
		t.Fatalf("BroadcastSkillLaunched targetIDs = %v, want to contain 2 and 3", ids)
	}
}

// mutableKnown is effectsKnown with a swappable roster, so a test can prove
// a value resolved once at Launch survives unchanged through Hit even if
// the underlying known-creature set has since moved on.
type mutableKnown struct {
	creatures []skilltarget.Creature
}

func (k *mutableKnown) ForEachKnownCreatureInRadius(anchor skilltarget.Creature, _ int, fn func(skilltarget.Creature)) {
	for _, c := range k.creatures {
		if c.ObjectID() == anchor.ObjectID() {
			continue
		}
		fn(c)
	}
}

// TestAIControllerCastReusesLaunchResolvedTargetsAtHit verifies Hit applies
// effects to the exact target set Launch already resolved and broadcast,
// not a fresh resolution 400ms later — matching CreatureCast.java's
// `_targets` field, assigned once in onMagicLaunch (:232) and read again
// unchanged by onMagicHitTimer's callSkill (:291, NpcCast.java:52). A
// bystander that leaves the known set between Launch and Hit must still be
// affected, because the set was already frozen at Launch.
func TestAIControllerCastReusesLaunchResolvedTargetsAtHit(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetArea
	def.Offensive = true
	def.Radius = 900
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{skillTypes: []string{"DUMMYCAST"}}
	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	selected := &fakeCastCreature{id: 2, x: 10, category: skilltarget.CategoryPlayable}
	bystander := &fakeCastCreature{id: 3, x: 20, category: skilltarget.CategoryPlayable}

	known := &mutableKnown{creatures: []skilltarget.Creature{caster, selected, bystander}}
	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     EffectHandlers{Targets: skilltarget.NewRegistry(known), Skills: handlerskill.NewRegistry(rec)},
		Caster:      caster,
	}

	ai.Cast(selected, ref)
	clock.fire(125 * time.Millisecond) // Launch — resolves & broadcasts [selected, bystander]

	if len(caster.skillLaunchedCalls) != 1 || len(caster.skillLaunchedCalls[0].targetIDs) != 2 {
		t.Fatalf("BroadcastSkillLaunched calls = %+v, want 1 call with 2 targets", caster.skillLaunchedCalls)
	}

	// bystander leaves the known set entirely before Hit fires — a fresh
	// resolution at Hit would miss it.
	known.creatures = []skilltarget.Creature{caster, selected}

	clock.fire(400 * time.Millisecond) // Hit

	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls = %d, want 1", len(rec.calls))
	}
	if len(rec.calls[0].Targets) != 2 {
		t.Fatalf("effect targets = %v, want 2 (the launch-frozen set, bystander included despite leaving known)", rec.calls[0].Targets)
	}
}

// TestAIControllerCastBroadcastsEmptyTargetListWhenLaunchResolutionFails
// verifies that when launch-time target resolution fails (no registered
// target handler here), MagicSkillLaunched broadcasts the empty list
// rather than synthesizing the single preselected target — matching
// CreatureCast.java:232-234's unconditional broadcast of whatever
// getTargetList returned, empty included, with no fallback.
func TestAIControllerCastBroadcastsEmptyTargetListWhenLaunchResolutionFails(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	target := &fakeCastCreature{id: 2, category: skilltarget.CategoryAttackable}

	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     EffectHandlers{}, // no Targets registry: resolution always fails
		Caster:      caster,
	}

	ai.Cast(target, ref)
	clock.fire(125 * time.Millisecond) // Launch

	if len(caster.skillLaunchedCalls) != 1 {
		t.Fatalf("BroadcastSkillLaunched calls = %d, want 1", len(caster.skillLaunchedCalls))
	}
	if ids := caster.skillLaunchedCalls[0].targetIDs; len(ids) != 0 {
		t.Fatalf("BroadcastSkillLaunched targetIDs = %v, want empty (no synthesized fallback target)", ids)
	}
}

// TestAIControllerCastBroadcastsSkillCanceledOnLaunchAbort verifies an
// aborted cast broadcasts MagicSkillCanceled, closing the loop this PR
// opened by making cast start observable — matching CreatureCast.stop()'s
// `if (isCastingNow()) _actor.broadcastPacket(new
// MagicSkillCanceled(...))` (CreatureCast.java:416-419), the common exit
// every abort path (Launch revalidation failure here; also insufficient
// MP/HP at Hit and a damage-break interrupt) routes through.
func TestAIControllerCastBroadcastsSkillCanceledOnLaunchAbort(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl := NewController(scalingActor())
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"
	def.EffectRange = 100

	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Caster:      caster,
	}

	ai.Cast(&fakeCastCreature{id: 2, x: 200, category: skilltarget.CategoryAttackable}, ref)
	clock.fire(125 * time.Millisecond) // Launch — RevalidateLaunch rejects (too far), aborts

	if len(caster.skillCanceledObjects) != 1 || caster.skillCanceledObjects[0] != 1 {
		t.Fatalf("BroadcastSkillCanceled objects = %v, want [1] (the caster's own id)", caster.skillCanceledObjects)
	}
	if len(caster.skillLaunchedCalls) != 0 {
		t.Fatal("BroadcastSkillLaunched called on an aborted launch")
	}
}

func TestAIControllerCastReportsLaunchAbort(t *testing.T) {
	clock := &fakeCastClock{}
	ctrl := NewController(scalingActor())
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"
	def.EffectRange = 100

	var got LaunchAbortReason
	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Caster:      &fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable},
		OnLaunchAbort: func(reason LaunchAbortReason) {
			got = reason
		},
	}

	ai.Cast(&fakeCastCreature{id: 2, x: 200, category: skilltarget.CategoryAttackable}, ref)
	clock.fire(125 * time.Millisecond)

	if got != LaunchAbortTooFar {
		t.Fatalf("launch abort = %v, want LaunchAbortTooFar", got)
	}
}

// TestAIControllerCastReportsHitResult verifies the Hit-phase EffectResult
// reaches OnHitResult instead of being discarded (issue 1572: a summon's
// failed-skill roll never notified the owner because AIController's Hit hook
// dropped ApplyResolvedEffectsResult's return value). Network wiring uses
// this hook to forward the result to the summon's owner, mirroring
// Summon.sendPacket's owner-forward; NPC casters simply leave it unset.
func TestAIControllerCastReportsHitResult(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{result: handlerskill.Result{AttackFailed: 1}}
	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	target := &fakeCastCreature{id: 2, category: skilltarget.CategoryAttackable}

	var got EffectResult
	var calls int
	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     newEffectHandlers(effectsKnown{}, "DUMMYCAST", rec),
		Caster:      caster,
		OnHitResult: func(result EffectResult) {
			got = result
			calls++
		},
	}

	ai.Cast(target, ref)
	clock.fire(125 * time.Millisecond) // Launch
	clock.fire(400 * time.Millisecond) // Hit

	if calls != 1 {
		t.Fatalf("OnHitResult calls = %d, want 1", calls)
	}
	if got.AttackFailed != 1 {
		t.Fatalf("OnHitResult AttackFailed = %d, want 1", got.AttackFailed)
	}
}

// TestAIControllerCastSkipsEffectsForFusionSkill matches
// CreatureCast.doFusionCast (CreatureCast.java:81-84), an empty stub for
// every non-player caster ("Non-Player Creatures cannot use FUSION or
// SIGNETS") — AIController drives exactly that non-player-initiated path, so
// a FUSION-skillType Hit must never reach the effect handlers, unlike a
// same-shaped non-FUSION skill.
func TestAIControllerCastSkipsEffectsForFusionSkill(t *testing.T) {
	clock := &fakeCastClock{}
	actor := scalingActor()
	ctrl := NewController(actor)
	ctrl.afterFunc = clock.AfterFunc

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "FUSION"

	rec := &recordingSkillHandler{}
	caster := &fakeBroadcastingCaster{fakeCastCreature: fakeCastCreature{id: 1, category: skilltarget.CategoryAttackable}}
	target := &fakeCastCreature{id: 2, category: skilltarget.CategoryAttackable}

	ai := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     newEffectHandlers(effectsKnown{}, "FUSION", rec),
		Caster:      caster,
	}

	ai.Cast(target, ref)

	// buildPlan skips atkSpd scaling for FUSION (controller.go:533-534,
	// matching PlayerCast.doFusionCast's raw getHitTime() read), so unlike
	// the atkSpd-scaled 125/400 choreography in the non-FUSION Hit test,
	// scalingDef's raw 1500ms HitTime yields LaunchDelay 1100ms, HitDelay
	// 400ms (controller.go:564-566).
	clock.fire(1100 * time.Millisecond)
	clock.fire(400 * time.Millisecond)

	if len(caster.skillLaunchedCalls) != 1 {
		t.Fatalf("BroadcastSkillLaunched calls = %d, want 1 (Hit phase must actually run for this test to prove anything)", len(caster.skillLaunchedCalls))
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls after FUSION Hit phase = %d, want 0 (CreatureCast.doFusionCast is a no-op)", len(rec.calls))
	}
}

func TestAIControllerCastNoOpsForUnknownSkill(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor)
	ai := &AIController{Controller: ctrl, Definitions: fakeDefinitions{}}
	target := &fakeCastCreature{id: 2}

	ai.Cast(target, modelskill.Ref{ID: 999})

	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after casting an unresolvable skill ref")
	}
}

type fakeDefinitions map[modelskill.Ref]modelskill.Definition

func (f fakeDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	d, ok := f[ref]
	return d, ok
}

// fakeCastCreature satisfies both attackable.Combatant (the ai package's
// desire/target surface) and skilltarget.Creature (the target-resolution
// surface ApplyEffects needs), so the same fake can stand in for an
// AIController's target on both sides of the bridge it builds.
type fakeCastCreature struct {
	id       int32
	x, y, z  int
	dead     bool
	category skilltarget.Category
}

func (f *fakeCastCreature) ObjectID() int32                                    { return f.id }
func (f *fakeCastCreature) Position() (int, int, int)                          { return f.x, f.y, f.z }
func (f *fakeCastCreature) Heading() int                                       { return 0 }
func (f *fakeCastCreature) Dead() bool                                         { return f.dead }
func (f *fakeCastCreature) Category() skilltarget.Category                     { return f.category }
func (f *fakeCastCreature) SiegeGuard() bool                                   { return false }
func (f *fakeCastCreature) AlikeDead() bool                                    { return f.dead }
func (f *fakeCastCreature) AttackableBy(skilltarget.Creature) bool             { return true }
func (f *fakeCastCreature) AttackableWithoutForceBy(skilltarget.Creature) bool { return true }

var _ attackable.Combatant = (*fakeCastCreature)(nil)
var _ skilltarget.Creature = (*fakeCastCreature)(nil)
var _ Target = (*fakeCastCreature)(nil)

type skillUseCall struct {
	targetID                  int32
	targetX, targetY, targetZ int
	skillID, level            int32
	hitTime, reuseDelay       int
}

type skillLaunchedCall struct {
	skillID, level int32
	targetIDs      []int32
}

// fakeBroadcastingCaster is a fakeCastCreature that also satisfies
// magicCastBroadcaster, recording every AI-cast broadcast call so tests can
// assert the Launch/Hit packet sequence AIController.Cast wires.
type fakeBroadcastingCaster struct {
	fakeCastCreature
	skillUseCalls        []skillUseCall
	skillLaunchedCalls   []skillLaunchedCall
	skillCanceledObjects []int32
}

func (f *fakeBroadcastingCaster) BroadcastSkillUse(targetID int32, targetX, targetY, targetZ int, skillID, level int32, hitTime, reuseDelay int) error {
	f.skillUseCalls = append(f.skillUseCalls, skillUseCall{targetID, targetX, targetY, targetZ, skillID, level, hitTime, reuseDelay})
	return nil
}

func (f *fakeBroadcastingCaster) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) error {
	f.skillLaunchedCalls = append(f.skillLaunchedCalls, skillLaunchedCall{skillID, level, targetIDs})
	return nil
}

func (f *fakeBroadcastingCaster) BroadcastSkillCanceled(objectID int32) error {
	f.skillCanceledObjects = append(f.skillCanceledObjects, objectID)
	return nil
}

var _ magicCastBroadcaster = (*fakeBroadcastingCaster)(nil)

// ---- from cubic_fire_test.go ----
func TestCubicGrantedLevel(t *testing.T) {
	tests := []struct {
		name string
		def  modelskill.Definition
		want int
	}{
		{"regular skill passes level through", modelskill.Definition{ID: 10, Level: 5}, 5},
		{"Life Cubic for Beginners forces level 8", modelskill.Definition{ID: 4338, Level: 1}, 8},
		{"enchanted level above 100 collapses via the reference formula", modelskill.Definition{ID: 10, Level: 121}, 11},
		{"non-exact-multiple truncates toward zero like Java int division", modelskill.Definition{ID: 10, Level: 125}, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CubicGrantedLevel(tt.def); got != tt.want {
				t.Fatalf("CubicGrantedLevel(%+v) = %d, want %d", tt.def, got, tt.want)
			}
		})
	}
}

type fakeCubicHealTarget struct {
	healable      bool
	effectiveness float64
	added         float64
}

func (f *fakeCubicHealTarget) ObjectID() int32           { return 1 }
func (f *fakeCubicHealTarget) Position() (int, int, int) { return 0, 0, 0 }
func (f *fakeCubicHealTarget) CanBeHealed() bool         { return f.healable }
func (f *fakeCubicHealTarget) AddHP(amount float64) float64 {
	f.added = amount
	return amount
}
func (f *fakeCubicHealTarget) HealEffectiveness() float64 { return f.effectiveness }

func TestApplyCubicHeal_FlatFormulaNoCasterStats(t *testing.T) {
	target := &fakeCubicHealTarget{healable: true, effectiveness: 150}
	if !ApplyCubicHeal(200, target) {
		t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
	}

	want := 200.0 * 150 / 100
	if target.added != want {
		t.Fatalf("AddHP amount = %v, want %v (power * effectiveness / 100, no caster stats)", target.added, want)
	}
}

func TestApplyCubicHeal_SkipsUnhealableTarget(t *testing.T) {
	target := &fakeCubicHealTarget{healable: false, effectiveness: 100}
	if ApplyCubicHeal(200, target) {
		t.Fatal("ApplyCubicHeal() = true for an unhealable target, want false")
	}
	if target.added != 0 {
		t.Fatalf("AddHP called on an unhealable target, amount = %v", target.added)
	}
}

// fakeCubicEffectCaster and fakeCubicEffectTarget are the minimal
// handlerskill.Actor + cast.Target surface ApplyCubicEffect's dispatch
// needs. fakeCubicEffectTarget deliberately does not implement
// checkSkillSuccess's skillSuccessSource probe, so an offensive continuous
// skill (DEBUFF/DOT/etc.) always fails its landing roll — matching
// Cubic.useContinuousSkill's calcCubicSkillSuccess()==false branch
// (Cubic.java:439-444), the case this test exercises.
type fakeCubicEffectCaster struct{ id int32 }

func (f *fakeCubicEffectCaster) ObjectID() int32           { return f.id }
func (f *fakeCubicEffectCaster) Position() (int, int, int) { return 0, 0, 0 }
func (f *fakeCubicEffectCaster) Dead() bool                { return false }

type fakeCubicEffectTarget struct {
	id   int32
	list *effect.List
}

func (f *fakeCubicEffectTarget) ObjectID() int32           { return f.id }
func (f *fakeCubicEffectTarget) Position() (int, int, int) { return 0, 0, 0 }
func (f *fakeCubicEffectTarget) Dead() bool                { return false }
func (f *fakeCubicEffectTarget) EffectList() *effect.List  { return f.list }

// TestApplyCubicEffect_FailedOffensiveContinuousRollReportsAttackFailed
// covers the issue-1570 gap: ApplyCubicEffect used to discard
// skills.UseResult's Result outright, so a failed offensive continuous
// landing roll never reached the caller and the cubic's owner never saw
// ATTACK_FAILED, unlike the reference's useContinuousSkill.
func TestApplyCubicEffect_FailedOffensiveContinuousRollReportsAttackFailed(t *testing.T) {
	registry := handlerskill.NewDefaultRegistry()
	caster := &fakeCubicEffectCaster{id: 1}
	target := &fakeCubicEffectTarget{id: 2, list: effect.NewList(nil)}

	def := modelskill.Definition{
		SkillType: "DEBUFF",
		Offensive: true,
		Debuff:    true,
		Effects:   []modelskill.EffectTemplate{{Name: "Buff", Time: 600}},
	}

	result := ApplyCubicEffect(registry, caster, def, target)

	if !result.Handled {
		t.Fatal("ApplyCubicEffect() Handled = false, want true (DEBUFF has a registered handler)")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("ApplyCubicEffect() AttackFailed = %d, want 1", result.AttackFailed)
	}
}

// fakeCubicFireOwner is a minimal CubicFireOwner + Target implementer for
// domain-level target-selection tests.
type fakeCubicFireOwner struct {
	objectID int32
	x, y, z  int
	target   world.Tracked
	rolls    []int
	rollIdx  int
	hp       int
	maxHP    float64
}

func (f *fakeCubicFireOwner) ObjectID() int32           { return f.objectID }
func (f *fakeCubicFireOwner) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeCubicFireOwner) Target() world.Tracked     { return f.target }
func (f *fakeCubicFireOwner) CurrentHP() int            { return f.hp }
func (f *fakeCubicFireOwner) MaxHPValue() float64       { return f.maxHP }
func (f *fakeCubicFireOwner) Roll(n int) int {
	if f.rollIdx >= len(f.rolls) {
		return 0
	}
	v := f.rolls[f.rollIdx]
	f.rollIdx++
	if v >= n && n > 0 {
		return n - 1
	}
	return v
}

type fakeCubicTarget struct {
	world.Presence
	objectID   int32
	x, y, z    int
	alikeDead  bool
	siegeGuard bool
}

func (f *fakeCubicTarget) ObjectID() int32           { return f.objectID }
func (f *fakeCubicTarget) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeCubicTarget) AlikeDead() bool           { return f.alikeDead }
func (f *fakeCubicTarget) SiegeGuard() bool          { return f.siegeGuard }

func TestDecideCubicFire_RejectsWhenActivationRollFails(t *testing.T) {
	owner := &fakeCubicFireOwner{rolls: []int{99}} // roll(100)=99 >= chance(30)
	_, _, ok := DecideCubicFire(owner, []int{4049}, 30)
	if ok {
		t.Fatal("DecideCubicFire() = true despite a failed activation roll")
	}
}

func TestDecideCubicFire_PicksTargetAndSkillOnSuccess(t *testing.T) {
	target := &fakeCubicTarget{objectID: 2, x: 100, y: 0, z: 0}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
	skillID, got, ok := DecideCubicFire(owner, []int{4049, 4053}, 100)
	if !ok {
		t.Fatal("DecideCubicFire() = false, want true (roll passes, target in range)")
	}
	if skillID != 4049 {
		t.Fatalf("skillID = %d, want 4049 (first roll picks index 0)", skillID)
	}
	if got.ObjectID() != target.ObjectID() {
		t.Fatalf("target ObjectID = %d, want %d", got.ObjectID(), target.ObjectID())
	}
}

func TestDecideCubicFire_RejectsOutOfRangeTarget(t *testing.T) {
	target := &fakeCubicTarget{objectID: 2, x: 10000, y: 0, z: 0}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
	_, _, ok := DecideCubicFire(owner, []int{4049}, 100)
	if ok {
		t.Fatal("DecideCubicFire() = true for a target far outside cubicMaxMagicRange")
	}
}

func TestDecideCubicFire_RejectsDeadTarget(t *testing.T) {
	target := &fakeCubicTarget{objectID: 2, alikeDead: true}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
	_, _, ok := DecideCubicFire(owner, []int{4049}, 100)
	if ok {
		t.Fatal("DecideCubicFire() = true for an already-dead target")
	}
}

func TestDecideLifeCubicTarget_SkipsWhenAtFullHP(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 100, maxHP: 100}
	_, ok := DecideLifeCubicTarget(owner)
	if ok {
		t.Fatal("DecideLifeCubicTarget() = true at full HP, want false")
	}
}

func TestDecideLifeCubicTarget_HealsSelfWhenRollPasses(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 1, maxHP: 1000, rolls: []int{0}}
	target, ok := DecideLifeCubicTarget(owner)
	if !ok {
		t.Fatal("DecideLifeCubicTarget() = false despite low HP and a passing roll")
	}
	if target.ObjectID() != owner.ObjectID() {
		t.Fatalf("target ObjectID = %d, want owner's own %d (no-party fallback)", target.ObjectID(), owner.ObjectID())
	}
}

// ---- from effects_test.go ----
// effectsActor is a minimal skilltarget.Creature usable as a non-player
// caster, proving the resolution path ApplyEffects drives doesn't require
// the live-player packet-handling type the player cast flow uses.
type effectsActor struct {
	id       int32
	x, y, z  int
	category skilltarget.Category
	dead     bool
	corpse   bool
	monster  bool
	summon   *effectsActor
}

type pvpEffectsActor struct {
	effectsActor
	calls []pvpSkillCall
}

type pvpSkillCall struct {
	targets   []creature.DeathActor
	offensive bool
	skillType string
}

func (a *pvpEffectsActor) NotePvPSkillTargets(targets []creature.DeathActor, offensive bool, skillType string) {
	a.calls = append(a.calls, pvpSkillCall{targets: append([]creature.DeathActor(nil), targets...), offensive: offensive, skillType: skillType})
}

func (a *effectsActor) ObjectID() int32                { return a.id }
func (a *effectsActor) Position() (int, int, int)      { return a.x, a.y, a.z }
func (a *effectsActor) Heading() int                   { return 0 }
func (a *effectsActor) Dead() bool                     { return a.dead }
func (a *effectsActor) Category() skilltarget.Category { return a.category }

func (a *effectsActor) AttackableBy(skilltarget.Creature) bool             { return true }
func (a *effectsActor) AttackableWithoutForceBy(skilltarget.Creature) bool { return true }
func (a *effectsActor) HasCorpse() bool                                    { return a.corpse }
func (a *effectsActor) MonsterKind() bool                                  { return a.monster }

func (a *effectsActor) Summon() (skilltarget.Creature, bool) {
	if a.summon == nil {
		return nil, false
	}
	return a.summon, true
}

// effectsKnown is a fixed roster used as the radius-scan source for
// area/aura target handlers under test.
type effectsKnown []skilltarget.Creature

func (k effectsKnown) ForEachKnownCreatureInRadius(anchor skilltarget.Creature, _ int, fn func(skilltarget.Creature)) {
	for _, c := range k {
		if c.ObjectID() == anchor.ObjectID() {
			continue
		}
		fn(c)
	}
}

// recordingSkillHandler records every Cast it receives instead of applying
// any actual skill logic, so tests can assert on exactly what ApplyEffects
// resolved and handed off.
type recordingSkillHandler struct {
	skillTypes []string
	calls      []handlerskill.Cast
	result     handlerskill.Result
}

func (h *recordingSkillHandler) Types() []string { return h.skillTypes }

func (h *recordingSkillHandler) Use(c handlerskill.Cast) { h.calls = append(h.calls, c) }

func (h *recordingSkillHandler) UseResult(c handlerskill.Cast) handlerskill.Result {
	h.Use(c)
	return h.result
}

func newEffectHandlers(known skilltarget.Known, skillType string, rec *recordingSkillHandler) EffectHandlers {
	rec.skillTypes = []string{skillType}
	return EffectHandlers{
		Targets: skilltarget.NewRegistry(known),
		Skills:  handlerskill.NewRegistry(rec),
	}
}

func TestApplyEffectsResultCarriesSkillHandlerAttackFailed(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{result: handlerskill.Result{AttackFailed: 2}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if !result.Handled {
		t.Fatal("ApplyEffectsResult() handled = false, want true")
	}
	if result.AttackFailed != 2 {
		t.Fatalf("AttackFailed = %d, want 2", result.AttackFailed)
	}
}

func TestApplyEffectsResultCarriesCubicTargets(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	other := &effectsActor{id: 2, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{result: handlerskill.Result{
		CubicTargets:      []handlerskill.Actor{other},
		CubicAddedTargets: []handlerskill.Actor{other},
	}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if got := result.CubicTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicTargets = %v, want other", got)
	}
	if got := result.CubicAddedTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicAddedTargets = %v, want other", got)
	}
}

func TestApplyEffectsResultCarriesSkillHandlerCounterattack(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{result: handlerskill.Result{Counterattacks: []handlerskill.Counterattack{{
		AttackerID: 1, AttackerName: "Attacker", DefenderID: 2, DefenderName: "Defender",
	}}}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if got := result.Counterattacks; len(got) != 1 || got[0].AttackerName != "Attacker" || got[0].DefenderName != "Defender" {
		t.Fatalf("Counterattacks = %+v, want attacker and defender", got)
	}
}

func TestApplyEffectsNotifiesPvPStatusBeforeSkillHandling(t *testing.T) {
	caster := &pvpEffectsActor{effectsActor: effectsActor{id: 1, category: skilltarget.CategoryPlayable}}
	target := &effectsActor{id: 2, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetOne, Offensive: true, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, target, def) {
		t.Fatal("ApplyEffects() = false, want true")
	}
	if len(caster.calls) != 1 {
		t.Fatalf("PvP status calls = %d, want 1", len(caster.calls))
	}
	call := caster.calls[0]
	if !call.offensive || call.skillType != "DUMMY" || len(call.targets) != 1 || call.targets[0] != target {
		t.Fatalf("PvP status call = %+v, want offensive selected target", call)
	}
}

func TestApplyEffectsAreaTargetReachesEveryAffectedCreature(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryAttackable}
	selected := &effectsActor{id: 2, x: 10, category: skilltarget.CategoryPlayable}
	bystander := &effectsActor{id: 3, x: 20, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{caster, selected, bystander}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetArea, Offensive: true, Radius: 900, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, selected, def) {
		t.Fatal("ApplyEffects(area) = false, want true")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls = %d, want 1", len(rec.calls))
	}
	if got := rec.calls[0].Caster; got != any(caster) {
		t.Fatalf("recorded caster = %v, want %v", got, caster)
	}
	if len(rec.calls[0].Targets) != 2 {
		t.Fatalf("recorded targets = %d, want 2 (selected + bystander)", len(rec.calls[0].Targets))
	}
}

func TestApplyEffectsAuraTargetSweepsRadiusAroundCaster(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryAttackable}
	nearby := &effectsActor{id: 2, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{caster, nearby}, "DUMMY", rec)
	def := modelskill.Definition{ID: 101, Target: modelskill.TargetAura, Radius: 300, SkillType: "DUMMY"}

	// Aura skills have no selected target: the caster is both the anchor
	// and the resolved target.
	if !ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(aura) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 {
		t.Fatalf("recorded call = %+v, want one call with one target", rec.calls)
	}
	if rec.calls[0].Targets[0] != any(nearby) {
		t.Fatalf("recorded target = %v, want %v", rec.calls[0].Targets[0], nearby)
	}
}

func TestApplyEffectsCorpseMobTargetRequiresPendingCorpse(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	corpse := &effectsActor{id: 2, category: skilltarget.CategoryAttackable, dead: true, corpse: true, monster: true}
	live := &effectsActor{id: 3, category: skilltarget.CategoryAttackable, corpse: false}
	def := modelskill.Definition{ID: 102, Target: modelskill.TargetCorpseMob, SkillType: "SWEEP"}

	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "SWEEP", rec)
	if !ApplyEffects(handlers, caster, corpse, def) {
		t.Fatal("ApplyEffects(corpse mob, has corpse) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(corpse) {
		t.Fatalf("recorded call = %+v, want one call targeting the corpse", rec.calls)
	}

	rec2 := &recordingSkillHandler{}
	handlers2 := newEffectHandlers(effectsKnown{}, "SWEEP", rec2)
	if ApplyEffects(handlers2, caster, live, def) {
		t.Fatal("ApplyEffects(corpse mob, no corpse) = true, want false")
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 for a target with no corpse", len(rec2.calls))
	}
}

func TestApplyEffectsSummonTargetResolvesCasterOwnedSummon(t *testing.T) {
	summon := &effectsActor{id: 2, category: skilltarget.CategoryPlayable}
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable, summon: summon}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 103, Target: modelskill.TargetSummon, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(summon) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(summon) {
		t.Fatalf("recorded call = %+v, want one call targeting the caster's summon", rec.calls)
	}

	// A caster without a summon must not reach the skill handler at all.
	rec2 := &recordingSkillHandler{}
	handlers2 := newEffectHandlers(effectsKnown{}, "DUMMY", rec2)
	summonless := &effectsActor{id: 4, category: skilltarget.CategoryPlayable}
	if ApplyEffects(handlers2, summonless, nil, def) {
		t.Fatal("ApplyEffects(summon, no summon) = true, want false")
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 for a caster without a summon", len(rec2.calls))
	}
}

func TestApplyEffectsUnresolvedTargetTypeIsNoop(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 104, Target: modelskill.TargetEnemySummon, SkillType: "DUMMY"}

	if ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(unregistered target type) = true, want false")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0", len(rec.calls))
	}
}

func TestApplyEffectsNilRegistriesAreNoop(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	def := modelskill.Definition{ID: 105, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	if ApplyEffects(EffectHandlers{}, caster, nil, def) {
		t.Fatal("ApplyEffects(zero-value handlers) = true, want false")
	}
	if ApplyEffects(EffectHandlers{}, nil, nil, def) {
		t.Fatal("ApplyEffects(nil caster) = true, want false")
	}
}

// nonCreatureSelection is a world.Tracked value that does not satisfy
// skilltarget.Creature — a door, static object, or similar world object a
// player can select but that carries no combat-relevant state.
type nonCreatureSelection struct {
	world.Presence
	id int32
}

func (s *nonCreatureSelection) ObjectID() int32 { return s.id }

var _ world.Tracked = (*nonCreatureSelection)(nil)
var _ Target = (*nonCreatureSelection)(nil)

// TestApplyEffectsRejectsNonCreatureSelection pins the quirk #1502 preserves:
// typing Selected to world.Tracked doesn't tighten what TargetOne admits at
// selection time (a door still satisfies cast.Target, so SelectTarget still
// accepts it), but resolveAffected's skilltarget.Creature narrowing still
// rejects it before any skill handler runs — the same branch as before the
// caster/selection types were tightened.
func TestApplyEffectsRejectsNonCreatureSelection(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	door := &nonCreatureSelection{id: 2}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 106, Target: modelskill.TargetOne, SkillType: "DUMMY"}

	if _, ok := SelectTarget(caster, door, def); !ok {
		t.Fatal("SelectTarget(door) ok = false, want true: a non-creature Tracked value still selects")
	}
	if ApplyEffects(handlers, caster, door, def) {
		t.Fatal("ApplyEffects(door selection) = true, want false")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 (door never reaches a skill handler)", len(rec.calls))
	}
}

// ---- from launch_revalidate_test.go ----
// launchActor is a minimal fixture implementing every optional surface
// RevalidateLaunch's gates consult, each independently controllable so
// tests can isolate one gate at a time.
type launchActor struct {
	id          int32
	x, y, z     int
	category    skilltarget.Category
	radius      float64
	sees        bool
	knows       bool
	inPeaceZone bool
}

func (a *launchActor) ObjectID() int32                        { return a.id }
func (a *launchActor) Position() (int, int, int)              { return a.x, a.y, a.z }
func (a *launchActor) Heading() int                           { return 0 }
func (a *launchActor) Dead() bool                             { return false }
func (a *launchActor) Category() skilltarget.Category         { return a.category }
func (a *launchActor) CollisionRadius() float64               { return a.radius }
func (a *launchActor) SiegeGuard() bool                       { return false }
func (a *launchActor) AlikeDead() bool                        { return false }
func (a *launchActor) CanSeeTarget(skilltarget.Creature) bool { return a.sees }
func (a *launchActor) Knows(attackable.Combatant) bool        { return a.knows }
func (a *launchActor) EffectRangeInPeaceZone(x, y, z, effectRange int) bool {
	return a.inPeaceZone
}

func TestRevalidateLaunchSelfTargetSkipsEveryGate(t *testing.T) {
	caster := &launchActor{id: 1, knows: false, inPeaceZone: true, sees: false}
	def := modelskill.Definition{Offensive: true, Radius: 900, EffectRange: 50}

	if got := RevalidateLaunch(caster, caster, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(self) = %v, want LaunchAbortNone", got)
	}
}

func TestFusionChannelValidRequiresRangeAndLineOfSight(t *testing.T) {
	caster := &launchActor{id: 1, sees: true}
	target := &launchActor{id: 2, x: 400}
	if !FusionChannelValid(caster, target, 400) {
		t.Fatal("FusionChannelValid() = false within range and line of sight")
	}
	target.x = 401
	if FusionChannelValid(caster, target, 400) {
		t.Fatal("FusionChannelValid() = true out of range")
	}
	target.x = 400
	caster.sees = false
	if FusionChannelValid(caster, target, 400) {
		t.Fatal("FusionChannelValid() = true without line of sight")
	}
}

func TestRevalidateLaunchTargetLost(t *testing.T) {
	caster := &launchActor{id: 1, knows: false}
	target := &launchActor{id: 2}
	def := modelskill.Definition{SkillType: "PDAM"}

	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortTargetLost {
		t.Fatalf("RevalidateLaunch() = %v, want LaunchAbortTargetLost", got)
	}
}

func TestRevalidateLaunchSummonFriendBypassesTargetLost(t *testing.T) {
	caster := &launchActor{id: 1, knows: false}
	target := &launchActor{id: 2}
	def := modelskill.Definition{SkillType: "SUMMON_FRIEND"}

	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(SUMMON_FRIEND) = %v, want LaunchAbortNone", got)
	}
}

func TestRevalidateLaunchEscapeRange(t *testing.T) {
	tests := []struct {
		name string
		def  modelskill.Definition
		dist int
		want LaunchAbortReason
	}{
		{
			name: "effect range too far",
			def:  modelskill.Definition{EffectRange: 100},
			dist: 200,
			want: LaunchAbortTooFar,
		},
		{
			name: "effect range within",
			def:  modelskill.Definition{EffectRange: 100},
			dist: 50,
			want: LaunchAbortNone,
		},
		{
			name: "radius fallback when no cast range and radius over 80",
			def:  modelskill.Definition{CastRange: 0, Radius: 200},
			dist: 300,
			want: LaunchAbortTooFar,
		},
		{
			name: "no escape range when cast range is set",
			def:  modelskill.Definition{CastRange: 40, Radius: 200},
			dist: 10000,
			want: LaunchAbortNone,
		},
		{
			name: "no escape range when radius is default 80",
			def:  modelskill.Definition{Radius: 80},
			dist: 10000,
			want: LaunchAbortNone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := &launchActor{id: 1, knows: true, sees: true}
			target := &launchActor{id: 2, x: tt.dist, knows: true, sees: true}
			if got := RevalidateLaunch(caster, target, tt.def); got != tt.want {
				t.Fatalf("RevalidateLaunch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRevalidateLaunchEscapeRangeIncludesCollisionRadii(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, radius: 20}
	target := &launchActor{id: 2, x: 115, knows: true, sees: true, radius: 20}
	def := modelskill.Definition{EffectRange: 100}

	// Raw distance (115) exceeds the range (100), but the actors' combined
	// collision radii (40) cover the gap, matching MathUtil.checkIfInRange.
	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch() = %v, want LaunchAbortNone (collision radii close the gap)", got)
	}
}

func TestRevalidateLaunchLineOfSight(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: false}
	target := &launchActor{id: 2}

	blocked := modelskill.Definition{Radius: 100}
	if got := RevalidateLaunch(caster, target, blocked); got != LaunchAbortNoLineOfSight {
		t.Fatalf("RevalidateLaunch(radius>0, blocked) = %v, want LaunchAbortNoLineOfSight", got)
	}

	noRadius := modelskill.Definition{Radius: 0}
	if got := RevalidateLaunch(caster, target, noRadius); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(radius=0, blocked LOS) = %v, want LaunchAbortNone (gate skipped)", got)
	}
}

func TestRevalidateLaunchPeaceZone(t *testing.T) {
	def := modelskill.Definition{Offensive: true, Radius: 0}

	casterInZone := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable, inPeaceZone: true}
	target := &launchActor{id: 2, category: skilltarget.CategoryPlayable}
	if got := RevalidateLaunch(casterInZone, target, def); got != LaunchAbortCasterPeaceZone {
		t.Fatalf("RevalidateLaunch(caster in peace zone) = %v, want LaunchAbortCasterPeaceZone", got)
	}

	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable}
	targetInZone := &launchActor{id: 2, category: skilltarget.CategoryPlayable, inPeaceZone: true}
	if got := RevalidateLaunch(caster, targetInZone, def); got != LaunchAbortTargetPeaceZone {
		t.Fatalf("RevalidateLaunch(target in peace zone) = %v, want LaunchAbortTargetPeaceZone", got)
	}
}

func TestRevalidateLaunchSummonTargetInPeaceZone(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable}
	target, err := summon.NewPet(summon.PetConfig{ObjectID: 2})
	if err != nil {
		t.Fatal(err)
	}
	target.SetZones(launchZoneQuery(true))

	if got := RevalidateLaunch(caster, target, modelskill.Definition{Offensive: true}); got != LaunchAbortTargetPeaceZone {
		t.Fatalf("RevalidateLaunch(summon target in peace zone) = %v, want LaunchAbortTargetPeaceZone", got)
	}
}

type launchZoneQuery bool

func (q launchZoneQuery) EffectRangeInPeaceZone(_, _, _, _, _, _ int) bool { return bool(q) }

func TestRevalidateLaunchPeaceZoneOnlyGatesOffensivePlayableVsPlayable(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable, inPeaceZone: true}
	target := &launchActor{id: 2, category: skilltarget.CategoryPlayable}

	nonOffensive := modelskill.Definition{Offensive: false}
	if got := RevalidateLaunch(caster, target, nonOffensive); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(non-offensive) = %v, want LaunchAbortNone", got)
	}

	npcTarget := &launchActor{id: 3, category: skilltarget.CategoryAttackable}
	offensive := modelskill.Definition{Offensive: true}
	if got := RevalidateLaunch(caster, npcTarget, offensive); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch(caster peace zone, non-playable target) = %v, want LaunchAbortNone", got)
	}
}

func TestRevalidateLaunchAllGatesPass(t *testing.T) {
	caster := &launchActor{id: 1, knows: true, sees: true, category: skilltarget.CategoryPlayable}
	target := &launchActor{id: 2, category: skilltarget.CategoryPlayable}
	def := modelskill.Definition{Offensive: true, EffectRange: 100, Radius: 100}

	if got := RevalidateLaunch(caster, target, def); got != LaunchAbortNone {
		t.Fatalf("RevalidateLaunch() = %v, want LaunchAbortNone", got)
	}
}

// ---- from player_actor_test.go ----
// permissiveGeo is a test-only move.Geo that permits every move, needed
// only because creature.NewLive requires a non-nil Geo.
type permissiveGeo struct{}

func (permissiveGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (permissiveGeo) Height(x, y, z int) int16                { return int16(z) }
func (permissiveGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (permissiveGeo) Walkable(int, int, int) bool { return true }
func (permissiveGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func TestPlayerActorResourcesAndInventory(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	ch := &player.Character{ID: 1}
	ch.SetResourceValues(player.Resources{MaxHP: 12, CurrentHP: 12, MaxMP: 7, CurrentMP: 7})
	inv := itemcontainer.NewPlayerInventory(ch.ID, templates)
	inv.AddNew(57, 5, 100)
	ch.AttachRuntime(&player.Template{}, inv)

	actor := PlayerActor{Character: ch}
	actor.ReduceMP(9)
	actor.ReduceHP(20)

	resources := ch.ResourceValues()
	if resources.CurrentMP != 0 || resources.CurrentHP != 0 {
		t.Fatalf("resources = hp %.0f mp %.0f, want both clamped to 0", resources.CurrentHP, resources.CurrentMP)
	}
	if got := actor.ItemCount(57); got != 5 {
		t.Fatalf("ItemCount() = %d, want 5", got)
	}
	if !actor.ConsumeItem(57, 3) {
		t.Fatalf("ConsumeItem() = false, want true")
	}
	if got := actor.ItemCount(57); got != 2 {
		t.Fatalf("ItemCount() after consume = %d, want 2", got)
	}
}

// TestPlayerActorMPCostAppliesDanceSurcharge covers Java's
// CreatureStatus.getMpConsume: a dance/song skill's MP cost grows by
// def.NextDanceCost for each dance/song already active on the caster, and
// a non-dance skill never picks up the surcharge.
func TestPlayerActorMPCostAppliesDanceSurcharge(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	actor := PlayerActor{Character: ch}

	dance := modelskill.Definition{Dance: true, MPConsume: 10, NextDanceCost: 4}
	if got := actor.MPCost(dance); got != 10 {
		t.Fatalf("MPCost() with no active dances = %d, want 10", got)
	}

	ch.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1, Dance: true, Toggle: true}, Type: effect.TypeBuff})
	ch.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 2, Dance: true, Toggle: true}, Type: effect.TypeBuff})

	if got := actor.MPCost(dance); got != 18 {
		t.Fatalf("MPCost() with 2 active dances = %d, want 18 (10 + 2*4)", got)
	}

	nonDance := modelskill.Definition{MPConsume: 10, NextDanceCost: 4}
	if got := actor.MPCost(nonDance); got != 10 {
		t.Fatalf("MPCost() for non-dance skill = %d, want 10, unaffected by active dances", got)
	}
}

func TestPlayerActorMPCostAppliesSkillMPConsumeRates(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	owner := effect.ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1})
	ch.AddStatFuncs([]effect.Mod{
		{Stat: stat.MagicalMpConsumeRate, Op: effect.OpMul, Value: 0.9, Owner: owner},
		{Stat: stat.PhysicalMpConsumeRate, Op: effect.OpMul, Value: 3, Owner: owner},
		{Stat: stat.DanceMpConsumeRate, Op: effect.OpMul, Value: 2, Owner: owner},
	})
	actor := PlayerActor{Character: ch}

	for _, tt := range []struct {
		name string
		def  modelskill.Definition
		init int
		cost int
	}{
		{name: "magic", def: modelskill.Definition{Magic: true, MPInitialConsume: 11, MPConsume: 21}, init: 9, cost: 18},
		{name: "physical", def: modelskill.Definition{MPInitialConsume: 11, MPConsume: 21}, init: 33, cost: 63},
		{name: "dance takes precedence", def: modelskill.Definition{Dance: true, Magic: true, MPInitialConsume: 11, MPConsume: 21}, init: 22, cost: 42},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := actor.MPInitialCost(tt.def); got != tt.init {
				t.Fatalf("MPInitialCost() = %d, want %d", got, tt.init)
			}
			if got := actor.MPCost(tt.def); got != tt.cost {
				t.Fatalf("MPCost() = %d, want %d", got, tt.cost)
			}
		})
	}
}

// TestPlayerActorAllSkillsDisabledReflectsCrowdControl covers Java's
// Creature.isAllSkillsDisabled(): a live crowd-control state (here, Stun)
// blocks casting through the same allSkillsDisabler seam Controller.CanCast
// and Controller.Stop both probe, and EnableAllSkills stays a no-op since
// this port doesn't model the raw Duel-defeat lock.
func TestPlayerActorAllSkillsDisabledReflectsCrowdControl(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	actor := PlayerActor{Character: ch}

	if actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = true before any lock is active")
	}

	e := &effect.Effect{Skill: effect.Skill{ID: 1}, Type: effect.TypeBuff, Flag: effect.FlagStunned}
	ch.EffectList().Add(e)
	if !actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = false while stunned, want true")
	}

	actor.EnableAllSkills()
	if !actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = false after EnableAllSkills, want still true: it only clears the unmodeled raw Duel lock, not crowd control")
	}

	ch.EffectList().Remove(e)
	if actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = true after the stun effect was removed")
	}
}

func TestPlayerActorSkillReuseDelegatesToCharacter(t *testing.T) {
	ch := &player.Character{}
	actor := PlayerActor{Character: ch}
	ref := modelskill.Ref{ID: 10, Level: 2}
	key := int32(10*256 + 2)

	actor.AddSkillReuse(ref, key, time.Minute)

	if !actor.SkillDisabled(key) {
		t.Fatalf("SkillDisabled() = false, want true")
	}
}

func TestPlayerActorResourceAccessIsRaceFree(t *testing.T) {
	ch := &player.Character{
		ID: 1,
	}
	ch.SetResourceValues(player.Resources{MaxHP: 100000, CurrentHP: 100000, MaxMP: 100000, CurrentMP: 100000})
	ch.AttachRuntime(&player.Template{}, nil)
	actor := PlayerActor{Character: ch}

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ch.TakeDamage(1, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			actor.ReduceHP(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			actor.ReduceMP(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = actor.HP()
			_ = actor.MP()
			_ = ch.CurrentHP()
			_ = ch.CurrentMP()
		}
	}()

	wg.Wait()

	if got := ch.CurrentHP(); got <= 0 {
		t.Fatalf("CurrentHP() = %d, want still alive", got)
	}
	if got := ch.CurrentMP(); got <= 0 {
		t.Fatalf("CurrentMP() = %d, want MP remaining", got)
	}
}

// ---- from request_test.go ----
func TestStartPlayerSkillAcceptsKnownActiveSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(3, 1)
	target := &requestTarget{id: 20}
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{
		{ID: 3, Level: 1}: {
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 1200,
		},
	}

	started, err := StartPlayerSkill(PlayerSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Selected:    target,
		SkillID:     3,
		Definitions: defs,
		Ctrl:        true,
		Shift:       true,
	})
	if err != nil {
		t.Fatalf("StartPlayerSkill() error: %v", err)
	}
	if started.Definition.ID != 3 || started.Definition.Level != 1 {
		t.Fatalf("Definition = %+v, want skill 3/1", started.Definition)
	}
	if started.Target != target {
		t.Fatalf("Target = %v, want selected target", started.Target)
	}
	if !started.Ctrl || !started.Shift {
		t.Fatalf("Ctrl = %v, Shift = %v, want both true", started.Ctrl, started.Shift)
	}
	if started.Plan.HitTime != 500*time.Millisecond || started.Plan.ReuseDelay != 1200*time.Millisecond {
		t.Fatalf("Plan timing = hit %s reuse %s, want 500ms/1.2s", started.Plan.HitTime, started.Plan.ReuseDelay)
	}
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false, want started")
	}
}

func TestStartPlayerSkillKeepsTargetRejectionFromResolver(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(3, 1)
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{{ID: 3, Level: 1}: {
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
	}}

	started, err := StartPlayerSkill(PlayerSkillRequest{
		Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
		Selected: &requestTarget{id: 20}, SkillID: 3, Definitions: defs,
		ResolveTarget: func(Target, world.Tracked, modelskill.Definition, bool) (Target, skilltarget.CastRejection) {
			return nil, skilltarget.CastRejectInvalidTarget
		},
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("StartPlayerSkill() error = %v, want ErrInvalidTarget", err)
	}
	if got := started.Rejection; got != skilltarget.CastRejectInvalidTarget {
		t.Fatalf("StartedSkill.Rejection = %v, want CastRejectInvalidTarget", got)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller started after a target rejection")
	}
}

// TestStartPlayerSkillClearsRecentFakeDeath covers PlayerCast.doCast's
// unconditional _actor.clearRecentFakeDeath() (PlayerCast.java:181-185),
// which runs right after super.doCast() commits to the cast — at cast
// start, not cast finish. A rejected start (invalid target here) never
// reaches that line in Java either, so the grace must survive it.
func TestStartPlayerSkillClearsRecentFakeDeath(t *testing.T) {
	def := modelskill.Definition{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 1200,
	}

	t.Run("accepted start clears the grace", func(t *testing.T) {
		ch := newRequestCharacter(10)
		ch.SetSkillLevel(3, 1)
		ch.MarkRecentFakeDeath()
		if !ch.RecentFakeDeath() {
			t.Fatal("MarkRecentFakeDeath did not set the grace")
		}
		ctrl := NewController(&testActor{mp: 100, hp: 100})
		defs := requestDefinitions{{ID: 3, Level: 1}: def}

		if _, err := StartPlayerSkill(PlayerSkillRequest{
			Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
			Selected: &requestTarget{id: 20}, SkillID: 3, Definitions: defs,
		}); err != nil {
			t.Fatalf("StartPlayerSkill() error: %v", err)
		}
		if ch.RecentFakeDeath() {
			t.Fatal("RecentFakeDeath() = true after an accepted cast start, want cleared")
		}
	})

	t.Run("rejected start leaves the grace running", func(t *testing.T) {
		ch := newRequestCharacter(10)
		ch.SetSkillLevel(3, 1)
		ch.MarkRecentFakeDeath()
		ctrl := NewController(&testActor{mp: 100, hp: 100})
		defs := requestDefinitions{{ID: 3, Level: 1}: def}

		if _, err := StartPlayerSkill(PlayerSkillRequest{
			Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
			SkillID: 3, Definitions: defs,
		}); !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("StartPlayerSkill() error = %v, want ErrInvalidTarget", err)
		}
		if !ch.RecentFakeDeath() {
			t.Fatal("RecentFakeDeath() = false after a rejected cast start, want still running")
		}
	})

	t.Run("FUSION start leaves the grace running", func(t *testing.T) {
		fusionDef := def
		fusionDef.SkillType = "FUSION"
		ch := newRequestCharacter(10)
		ch.SetSkillLevel(3, 1)
		ch.MarkRecentFakeDeath()
		ctrl := NewController(&testActor{mp: 100, hp: 100})
		defs := requestDefinitions{{ID: 3, Level: 1}: fusionDef}

		if _, err := StartPlayerSkill(PlayerSkillRequest{
			Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
			Selected: &requestTarget{id: 20}, SkillID: 3, Definitions: defs,
		}); err != nil {
			t.Fatalf("StartPlayerSkill() error: %v", err)
		}
		// PlayerAI dispatches FUSION (and SIGNET_CASTTIME) to
		// doFusionCast instead of doCast (PlayerAI.java:299-301), which
		// never calls clearRecentFakeDeath.
		if !ch.RecentFakeDeath() {
			t.Fatal("RecentFakeDeath() = false after a FUSION cast start, want still running")
		}
	})
}

func TestStartPlayerSkillRejectsUnavailableSkill(t *testing.T) {
	active := modelskill.Definition{ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	inactive := active
	inactive.Activation = modelskill.ActivationPassive

	tests := []struct {
		name    string
		skillID int
		level   int
		dead    bool
		defs    requestDefinitions
	}{
		{name: "nonpositive request", skillID: 0, level: 1, defs: requestDefinitions{{ID: 3, Level: 1}: active}},
		{name: "dead caster", skillID: 3, level: 1, dead: true, defs: requestDefinitions{{ID: 3, Level: 1}: active}},
		{name: "unknown level", skillID: 3, defs: requestDefinitions{{ID: 3, Level: 1}: active}},
		{name: "missing definition", skillID: 3, level: 1, defs: requestDefinitions{}},
		{name: "inactive definition", skillID: 3, level: 1, defs: requestDefinitions{{ID: 3, Level: 1}: inactive}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newRequestCharacter(10)
			ch.SetSkillLevel(3, tt.level)
			if tt.dead {
				ch.MarkDead()
			}
			ctrl := NewController(&testActor{mp: 100, hp: 100})

			if _, err := StartPlayerSkill(PlayerSkillRequest{
				Now:         time.Unix(1000, 0),
				Controller:  ctrl,
				Caster:      ch,
				SkillID:     tt.skillID,
				Definitions: tt.defs,
			}); !errors.Is(err, ErrSkillUnavailable) {
				t.Fatalf("StartPlayerSkill() error = %v, want ErrSkillUnavailable", err)
			}
			if ctrl.CastingNow() {
				t.Fatal("controller CastingNow() = true after unavailable skill")
			}
		})
	}
}

func TestStartPlayerSkillRejectsInvalidTarget(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(3, 1)
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{
		{ID: 3, Level: 1}: {ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne},
	}

	started, err := StartPlayerSkill(PlayerSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		SkillID:     3,
		Definitions: defs,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("StartPlayerSkill() error = %v, want ErrInvalidTarget", err)
	}
	if started.Definition.ID != 3 || started.Target != nil {
		t.Fatalf("started = %+v, want definition with nil target", started)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after invalid target")
	}
}

func TestStartItemSkillAcceptsResolvedSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	target := &requestTarget{id: 20}
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	def := modelskill.Definition{
		ID: 7, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		StaticHitTime: true, HitTime: 800, StaticReuse: true, ReuseDelay: 0,
	}
	defs := requestDefinitions{{ID: 7, Level: 1}: def}

	// A caster with no learned skill level for 7 still starts the cast:
	// unlike StartPlayerSkill, the definition comes from the item, not the
	// caster's own skill list.
	started, err := StartItemSkill(ItemSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Selected:    target,
		Skill:       modelskill.Ref{ID: 7, Level: 1},
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("StartItemSkill() error: %v", err)
	}
	if started.Definition.ID != 7 || started.Definition.Level != 1 {
		t.Fatalf("Definition = %+v, want skill 7/1", started.Definition)
	}
	if started.Target != target {
		t.Fatalf("Target = %v, want selected target", started.Target)
	}
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false, want started")
	}
}

func TestStartItemSkillRejectsUnavailableSkill(t *testing.T) {
	active := modelskill.Definition{ID: 7, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	inactive := active
	inactive.Activation = modelskill.ActivationPassive

	tests := []struct {
		name  string
		dead  bool
		skill modelskill.Ref
		defs  requestDefinitions
	}{
		{name: "dead caster", dead: true, skill: modelskill.Ref{ID: 7, Level: 1}, defs: requestDefinitions{{ID: 7, Level: 1}: active}},
		{name: "missing definition", skill: modelskill.Ref{ID: 7, Level: 1}, defs: requestDefinitions{}},
		{name: "inactive definition", skill: modelskill.Ref{ID: 7, Level: 1}, defs: requestDefinitions{{ID: 7, Level: 1}: inactive}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newRequestCharacter(10)
			if tt.dead {
				ch.MarkDead()
			}
			ctrl := NewController(&testActor{mp: 100, hp: 100})

			if _, err := StartItemSkill(ItemSkillRequest{
				Now:         time.Unix(1000, 0),
				Controller:  ctrl,
				Caster:      ch,
				Skill:       tt.skill,
				Definitions: tt.defs,
			}); !errors.Is(err, ErrSkillUnavailable) {
				t.Fatalf("StartItemSkill() error = %v, want ErrSkillUnavailable", err)
			}
			if ctrl.CastingNow() {
				t.Fatal("controller CastingNow() = true after unavailable skill")
			}
		})
	}
}

func TestStartItemSkillRejectsInvalidTarget(t *testing.T) {
	ch := newRequestCharacter(10)
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{
		{ID: 7, Level: 1}: {ID: 7, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne},
	}

	started, err := StartItemSkill(ItemSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Skill:       modelskill.Ref{ID: 7, Level: 1},
		Definitions: defs,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("StartItemSkill() error = %v, want ErrInvalidTarget", err)
	}
	if started.Definition.ID != 7 || started.Target != nil {
		t.Fatalf("started = %+v, want definition with nil target", started)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after invalid target")
	}
}

func TestResolvePlayerToggleAcceptsKnownToggleSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(288, 1)
	defs := requestDefinitions{
		{ID: 288, Level: 1}: {ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf},
	}

	def, target, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     288,
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("ResolvePlayerToggle() error: %v", err)
	}
	if def.ID != 288 || def.Level != 1 {
		t.Fatalf("Definition = %+v, want skill 288/1", def)
	}
	if target != ch {
		t.Fatalf("Target = %v, want the caster (SELF target)", target)
	}
}

func TestResolvePlayerToggleRejectsUnavailableSkill(t *testing.T) {
	toggle := modelskill.Definition{ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf}
	active := toggle
	active.Activation = modelskill.ActivationActive

	tests := []struct {
		name    string
		skillID int
		level   int
		dead    bool
		defs    requestDefinitions
	}{
		{name: "nonpositive request", skillID: 0, level: 1, defs: requestDefinitions{{ID: 288, Level: 1}: toggle}},
		{name: "dead caster", skillID: 288, level: 1, dead: true, defs: requestDefinitions{{ID: 288, Level: 1}: toggle}},
		{name: "unknown level", skillID: 288, defs: requestDefinitions{{ID: 288, Level: 1}: toggle}},
		{name: "missing definition", skillID: 288, level: 1, defs: requestDefinitions{}},
		{name: "non-toggle definition", skillID: 288, level: 1, defs: requestDefinitions{{ID: 288, Level: 1}: active}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newRequestCharacter(10)
			ch.SetSkillLevel(288, tt.level)
			if tt.dead {
				ch.MarkDead()
			}

			if _, _, err := ResolvePlayerToggle(PlayerToggleRequest{
				Caster:      ch,
				SkillID:     tt.skillID,
				Definitions: tt.defs,
			}); !errors.Is(err, ErrSkillUnavailable) {
				t.Fatalf("ResolvePlayerToggle() error = %v, want ErrSkillUnavailable", err)
			}
		})
	}
}

func TestResolvePlayerToggleRejectsInvalidTarget(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(288, 1)
	defs := requestDefinitions{
		{ID: 288, Level: 1}: {ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetOne},
	}

	def, target, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     288,
		Definitions: defs,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("ResolvePlayerToggle() error = %v, want ErrInvalidTarget", err)
	}
	if def.ID != 288 || target != nil {
		t.Fatalf("resolved = %+v/%v, want definition with nil target", def, target)
	}
}

// TestResolvePlayerToggleAllowsFakeDeathSkillWhileFaking matches
// PlayerCast.canAttemptCast's exception: `if (_actor.isFakeDeath() &&
// skill.getId() != 60)` (PlayerCast.java:218-222) rejects every cast while
// fake-dead except skill 60 itself, so a faking player can always re-cast
// the toggle to un-fake.
func TestResolvePlayerToggleAllowsFakeDeathSkillWhileFaking(t *testing.T) {
	ch := requestCharacterFakingDeath(t)
	ch.SetSkillLevel(fakeDeathSkillID, 1)
	defs := requestDefinitions{
		{ID: fakeDeathSkillID, Level: 1}: {ID: fakeDeathSkillID, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf},
	}

	def, target, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     fakeDeathSkillID,
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("ResolvePlayerToggle() error: %v, want nil while faking for skill %d", err, fakeDeathSkillID)
	}
	if def.ID != fakeDeathSkillID || target != ch {
		t.Fatalf("resolved = %+v/%v, want skill %d targeting the caster", def, target, fakeDeathSkillID)
	}
}

// TestResolvePlayerToggleRejectsOtherSkillsWhileFakeDeath asserts the
// exception above is scoped to skill 60 only: any other toggle stays
// rejected while faking, matching the same Java condition.
func TestResolvePlayerToggleRejectsOtherSkillsWhileFakeDeath(t *testing.T) {
	ch := requestCharacterFakingDeath(t)
	ch.SetSkillLevel(288, 1)
	defs := requestDefinitions{
		{ID: 288, Level: 1}: {ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf},
	}

	if _, _, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     288,
		Definitions: defs,
	}); !errors.Is(err, ErrSkillUnavailable) {
		t.Fatalf("ResolvePlayerToggle() error = %v, want ErrSkillUnavailable while faking for a non-60 skill", err)
	}
}

func requestCharacterFakingDeath(t *testing.T) *player.Character {
	t.Helper()
	ch := newRequestCharacter(10)
	live, err := creature.NewLive(location.Location{}, 0, requestGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live

	e, err := effect.New(effect.Skill{ID: fakeDeathSkillID}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatalf("effect.New(FakeDeath) error: %v", err)
	}
	e.Effected = ch
	ch.EffectList().Add(e)
	return ch
}

type requestGeo struct{}

func (requestGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (requestGeo) Height(_, _, _ int) int16          { return 0 }
func (requestGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (requestGeo) Walkable(int, int, int) bool { return true }
func (requestGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type requestDefinitions map[modelskill.Ref]modelskill.Definition

func newRequestCharacter(id int32) *player.Character {
	ch := &player.Character{ID: id}
	ch.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100})
	return ch
}

func (d requestDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

type requestTarget struct {
	world.Presence
	id int32
}

func (t *requestTarget) ObjectID() int32 { return t.id }

// ---- from target_test.go ----
func TestSelectTarget(t *testing.T) {
	caster := &castTarget{id: 1}
	selected := &castTarget{id: 2}

	tests := []struct {
		name     string
		target   modelskill.Target
		selected world.Tracked
		want     Target
		wantOK   bool
	}{
		{name: "self", target: modelskill.TargetSelf, want: caster, wantOK: true},
		{name: "none", target: modelskill.TargetNone, selected: selected, want: caster, wantOK: true},
		{name: "ground", target: modelskill.TargetGround, selected: selected, want: caster, wantOK: true},
		{name: "one", target: modelskill.TargetOne, selected: selected, want: selected, wantOK: true},
		{name: "one no selection", target: modelskill.TargetOne, wantOK: false},
		{name: "unsupported", target: modelskill.TargetParty, selected: selected, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SelectTarget(caster, tt.selected, modelskill.Definition{Target: tt.target})
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("SelectTarget() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

type castTarget struct {
	world.Presence
	id int32
}

func (t *castTarget) ObjectID() int32 { return t.id }

// ---- from testfakes_test.go ----
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

// ---- from toggle_test.go ----
// def reproduces "Guard Stance" (skill 288, level 1): a toggle with an MP
// upkeep cost and no HP cost. hpDef adds a nonzero HP cost on top, matching
// how other toggles (e.g. Fake Death) mix both costs.
func toggleDef() modelskill.Definition {
	return modelskill.Definition{
		ID:         288,
		Level:      1,
		Activation: modelskill.ActivationToggle,
		MPConsume:  12,
		ReuseDelay: 0,
	}
}

func TestCanCastToggleOnlyChecksBlanketLockAndReuseDelay(t *testing.T) {
	def := toggleDef()
	actor := &testActor{mp: 0, hp: 0}
	if err := NewController(actor).CanCastToggle(def); err != nil {
		t.Fatalf("CanCastToggle() error = %v, want nil despite empty resources", err)
	}

	actor.disabledKeys = map[int32]bool{ReuseKey(def): true}
	if err := NewController(actor).CanCastToggle(def); !errors.Is(err, ErrSkillDisabled) {
		t.Fatalf("CanCastToggle() error = %v, want ErrSkillDisabled", err)
	}
}

// TestCanCastToggleRejectsAllSkillsDisabled covers Java's
// RequestMagicSkillUse.java:24-68: there is no toggle branch, so a toggle
// press goes through player.getAI().tryToCast just like any other skill and
// is rejected by PlayableAI.tryToCast's denyAiAction() check
// (PlayableAI.java:299-303) while the actor is CC'd — toggles get no
// exemption from the blanket lock.
func TestCanCastToggleRejectsAllSkillsDisabled(t *testing.T) {
	def := toggleDef()
	actor := &testActor{mp: 100, hp: 100, allDisabled: true}
	if err := NewController(actor).CanCastToggle(def); !errors.Is(err, ErrAllSkillsDisabled) {
		t.Fatalf("CanCastToggle() error = %v, want ErrAllSkillsDisabled", err)
	}
}

func TestCanCastToggleRejectsNonToggleSkill(t *testing.T) {
	def := toggleDef()
	def.Activation = modelskill.ActivationActive

	actor := &testActor{mp: 100, hp: 100}
	if err := NewController(actor).CanCastToggle(def); err == nil {
		t.Fatal("CanCastToggle() error = nil, want an error for a non-toggle skill")
	}
}

func TestCastToggleDeactivatesAnAlreadyActiveInstanceAtNoCost(t *testing.T) {
	actor := &testActor{mp: 5, hp: 5}
	def := toggleDef()
	def.HPConsume = 3

	activated, err := NewController(actor).CastToggle(true, def)
	if err != nil {
		t.Fatalf("CastToggle() error: %v", err)
	}
	if activated {
		t.Fatal("CastToggle() activated = true, want false when already active")
	}
	if actor.mp != 5 || actor.hp != 5 {
		t.Fatalf("resources after deactivate = mp %d hp %d, want unchanged 5/5", actor.mp, actor.hp)
	}
}

func TestCastToggleActivatesAndPaysMPAndHP(t *testing.T) {
	actor := &testActor{mp: 20, hp: 10}
	def := toggleDef()
	def.HPConsume = 3

	activated, err := NewController(actor).CastToggle(false, def)
	if err != nil {
		t.Fatalf("CastToggle() error: %v", err)
	}
	if !activated {
		t.Fatal("CastToggle() activated = false, want true")
	}
	if actor.mp != 8 || actor.hp != 7 {
		t.Fatalf("resources after activate = mp %d hp %d, want 8/7", actor.mp, actor.hp)
	}
}

func TestCastToggleFailsWithoutConsumingWhenResourcesAreInsufficient(t *testing.T) {
	t.Run("mp", func(t *testing.T) {
		actor := &testActor{mp: 5, hp: 10}
		def := toggleDef()
		def.HPConsume = 3

		if _, err := NewController(actor).CastToggle(false, def); !errors.Is(err, ErrNotEnoughMP) {
			t.Fatalf("CastToggle() error = %v, want ErrNotEnoughMP", err)
		}
		if actor.mp != 5 || actor.hp != 10 {
			t.Fatalf("resources after failed activate = mp %d hp %d, want unchanged 5/10", actor.mp, actor.hp)
		}
	})

	// An HP shortfall is checked only after MP has already been paid, and
	// that MP is not refunded on failure — matching the reference
	// activation sequence's exact (non-transactional) ordering.
	t.Run("hp", func(t *testing.T) {
		actor := &testActor{mp: 20, hp: 2}
		def := toggleDef()
		def.HPConsume = 3

		if _, err := NewController(actor).CastToggle(false, def); !errors.Is(err, ErrNotEnoughHP) {
			t.Fatalf("CastToggle() error = %v, want ErrNotEnoughHP", err)
		}
		if actor.mp != 8 || actor.hp != 2 {
			t.Fatalf("resources after failed activate = mp %d hp %d, want mp already spent (8) and hp unchanged (2)", actor.mp, actor.hp)
		}
	})
}

func TestCastToggleNeverInstallsAReuseDelay(t *testing.T) {
	actor := &testActor{mp: 20, hp: 10}
	def := toggleDef()
	def.ReuseDelay = 60000

	if _, err := NewController(actor).CastToggle(false, def); err != nil {
		t.Fatalf("CastToggle() error: %v", err)
	}
	if len(actor.disabled) != 0 || len(actor.reuses) != 0 {
		t.Fatalf("cooldown state after activate = disabled %+v reuses %+v, want none", actor.disabled, actor.reuses)
	}
}
