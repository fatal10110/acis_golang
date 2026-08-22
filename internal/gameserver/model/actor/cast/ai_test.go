package cast

import (
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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
