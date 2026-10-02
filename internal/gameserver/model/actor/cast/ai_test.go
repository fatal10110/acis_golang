package cast

import (
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// ---- from ai_test.go ----
func TestAIControllerDisabledReflectsCastingNow(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor, nil)
	ai := &AIController{Controller: ctrl, Definitions: definitions()}

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
	ai := &AIController{Controller: NewController(actor, nil), Definitions: definitions()}

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
		Definitions: definitions(modelskill.Definition{ID: 5, Level: 1, CastRange: 600, HitTime: 1200}),
	}

	if got := ai.Range(ref); got != 600 {
		t.Fatalf("Range() = %d, want 600", got)
	}
	if !ai.StopsMovement(ref) {
		t.Fatal("StopsMovement() = false for a 1200ms hit time, want true")
	}

	shortRef := modelskill.Ref{ID: 6, Level: 1}
	ai.Definitions = definitions(modelskill.Definition{ID: 6, Level: 1, HitTime: 40})
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
	ctrl := NewController(actor, nil)
	ai := &AIController{Controller: ctrl, Definitions: definitions(def)}
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
	ctrl := NewController(actor, nil)
	ai := &AIController{Controller: ctrl, Definitions: definitions(def)}
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
	ctrl := NewController(actor, nil)
	ai := &AIController{Controller: ctrl, Definitions: definitions(def)}
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
	ai.Definitions = definitions(def)
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
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{}
	caster := &fakeCastCreature{id: 1, kind: modelactor.KindNPC}
	target := &fakeCastCreature{id: 2, kind: modelactor.KindNPC}

	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
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
	clock.advance(125 * time.Millisecond)
	clock.advance(400 * time.Millisecond)

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
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{}
	target := &fakeCastCreature{id: 2, x: 10, y: 20, z: 30, kind: modelactor.KindNPC}
	caster, events := newAICaster(t, 1, target)

	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects:     newEffectHandlers(effectsKnown{}, "DUMMYCAST", rec),
		Caster:      caster,
	}

	ai.Cast(target, ref)

	uses := event.Of[event.MagicSkillUse](events)
	if len(uses) != 1 {
		t.Fatalf("MagicSkillUse events at cast start = %d, want 1", len(uses))
	}
	// HitTime is the fixture's oracle-verified 525ms scaled hit time; the
	// reuse delay is the plan's own and not under test here.
	got := uses[0]
	got.ReuseDelay = 0
	want := event.MagicSkillUse{
		CasterID: 1, TargetID: 2, TargetAt: location.Location{X: 10, Y: 20, Z: 30},
		SkillID: int32(def.ID), Level: int32(def.Level), HitTime: 525,
	}
	if got != want {
		t.Fatalf("MagicSkillUse = %+v, want %+v", got, want)
	}
	if got := event.Count[event.SkillLaunched](events); got != 0 {
		t.Fatalf("SkillLaunched events before the Launch phase = %d, want 0", got)
	}

	clock.advance(125 * time.Millisecond) // Launch

	if got := event.Count[event.MagicSkillUse](events); got != 1 {
		t.Fatalf("MagicSkillUse events after Launch = %d, want still 1 (no re-broadcast)", got)
	}
	launched := event.Of[event.SkillLaunched](events)
	if len(launched) != 1 {
		t.Fatalf("SkillLaunched events after Launch = %d, want 1", len(launched))
	}
	if launched[0].SkillID != int32(def.ID) || launched[0].Level != int32(def.Level) {
		t.Fatalf("SkillLaunched skill = (%d,%d), want (%d,%d)", launched[0].SkillID, launched[0].Level, def.ID, def.Level)
	}
	if ids := launched[0].TargetIDs; len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("SkillLaunched TargetIDs = %v, want [2]", ids)
	}

	clock.advance(400 * time.Millisecond) // Hit

	if got := event.Count[event.SkillLaunched](events); got != 1 {
		t.Fatalf("SkillLaunched events after Hit = %d, want still 1 (no re-broadcast)", got)
	}
}

// TestAIControllerCastBroadcastsSkillLaunchedWithFullTargetList verifies
// MagicSkillLaunched carries every affected target from the launch-resolved
// target list, not just the AI's preselected target — matching
// CreatureCast.java:232-234's `_targets = _skill.getTargetList(_actor,
// _target)` recompute and `broadcastPacket(new MagicSkillLaunched(_actor,
// _skill, _targets))` broadcast of the full array.
func TestAIControllerCastBroadcastsSkillLaunchedWithFullTargetList(t *testing.T) {
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetArea
	def.Offensive = true
	def.Radius = 900
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{}
	selected := &fakeCastCreature{id: 2, x: 10, kind: modelactor.KindPlayer}
	bystander := &fakeCastCreature{id: 3, x: 20, kind: modelactor.KindPlayer}
	caster, events := newAICaster(t, 1, selected, bystander)

	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects:     newEffectHandlers(effectsKnown{caster, selected, bystander}, "DUMMYCAST", rec),
		Caster:      caster,
	}

	ai.Cast(selected, ref)
	clock.advance(125 * time.Millisecond) // Launch

	launched := event.Of[event.SkillLaunched](events)
	if len(launched) != 1 {
		t.Fatalf("SkillLaunched events after Launch = %d, want 1", len(launched))
	}
	ids := launched[0].TargetIDs
	if len(ids) != 2 {
		t.Fatalf("SkillLaunched TargetIDs = %v, want 2 ids (selected + bystander)", ids)
	}
	seen := map[int32]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[2] || !seen[3] {
		t.Fatalf("SkillLaunched TargetIDs = %v, want to contain 2 and 3", ids)
	}
}

// mutableKnown is effectsKnown with a swappable roster, so a test can prove
// a value resolved once at Launch survives unchanged through Hit even if
// the underlying known-creature set has since moved on.
type mutableKnown struct {
	creatures []skilltarget.Actor
}

func (k *mutableKnown) ForEachKnownCreatureInRadius(anchor skilltarget.Actor, _ int, fn func(skilltarget.Actor)) {
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
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetArea
	def.Offensive = true
	def.Radius = 900
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{skillTypes: []string{"DUMMYCAST"}}
	selected := &fakeCastCreature{id: 2, x: 10, kind: modelactor.KindPlayer}
	bystander := &fakeCastCreature{id: 3, x: 20, kind: modelactor.KindPlayer}
	caster, events := newAICaster(t, 1, selected, bystander)

	known := &mutableKnown{creatures: []skilltarget.Actor{caster, selected, bystander}}
	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects:     EffectHandlers{Targets: skilltarget.NewRegistry(known), Skills: handlerskill.NewRegistry(rec)},
		Caster:      caster,
	}

	ai.Cast(selected, ref)
	clock.advance(125 * time.Millisecond) // Launch — resolves & broadcasts [selected, bystander]

	if launched := event.Of[event.SkillLaunched](events); len(launched) != 1 || len(launched[0].TargetIDs) != 2 {
		t.Fatalf("SkillLaunched events = %+v, want 1 with 2 targets", launched)
	}

	// bystander leaves the known set entirely before Hit fires — a fresh
	// resolution at Hit would miss it.
	known.creatures = []skilltarget.Actor{caster, selected}

	clock.advance(400 * time.Millisecond) // Hit

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
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	target := &fakeCastCreature{id: 2, kind: modelactor.KindNPC}
	caster, events := newAICaster(t, 1, target)

	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects:     EffectHandlers{}, // no Targets registry: resolution always fails
		Caster:      caster,
	}

	ai.Cast(target, ref)
	clock.advance(125 * time.Millisecond) // Launch

	launched := event.Of[event.SkillLaunched](events)
	if len(launched) != 1 {
		t.Fatalf("SkillLaunched events = %d, want 1", len(launched))
	}
	if ids := launched[0].TargetIDs; len(ids) != 0 {
		t.Fatalf("SkillLaunched TargetIDs = %v, want empty (no synthesized fallback target)", ids)
	}
}

// TestAIControllerCastReportsAbortOnLaunchRevalidationFailure verifies an AI
// cast rejected at Launch reports CastAborted, the event the NPC's owner maps
// to MagicSkillCanceled — matching CreatureCast.stop()'s `if
// (isCastingNow()) _actor.broadcastPacket(new MagicSkillCanceled(...))`
// (CreatureCast.java:416-419), the common exit every abort path (Launch
// revalidation failure here; also insufficient MP/HP at Hit and a
// damage-break interrupt) routes through.
func TestAIControllerCastReportsAbortOnLaunchRevalidationFailure(t *testing.T) {
	clock := newCastClock()
	rec := &event.Recorder{}
	ctrl := NewController(scalingActor(), rec)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"
	def.EffectRange = 100

	target := &fakeCastCreature{id: 2, x: 200, kind: modelactor.KindNPC}
	caster, events := newAICaster(t, 1, target)
	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Caster:      caster,
	}

	ai.Cast(target, ref)
	clock.advance(125 * time.Millisecond) // Launch — RevalidateLaunch rejects (too far), aborts

	if got := event.Count[event.CastAborted](rec); got != 1 {
		t.Fatalf("CastAborted events = %d, want 1", got)
	}
	if got := event.Count[event.SkillLaunched](events); got != 0 {
		t.Fatalf("SkillLaunched events on an aborted launch = %d, want 0", got)
	}
}

func TestAIControllerCastReportsLaunchAbort(t *testing.T) {
	clock := newCastClock()
	ctrl := NewController(scalingActor(), nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"
	def.EffectRange = 100

	var got LaunchAbortReason
	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Caster:      &fakeCastCreature{id: 1, kind: modelactor.KindNPC},
		OnLaunchAbort: func(reason LaunchAbortReason) {
			got = reason
		},
	}

	ai.Cast(&fakeCastCreature{id: 2, x: 200, kind: modelactor.KindNPC}, ref)
	clock.advance(125 * time.Millisecond)

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
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "DUMMYCAST"

	rec := &recordingSkillHandler{result: handlerskill.Result{AttackFailed: 1}}
	target := &fakeCastCreature{id: 2, kind: modelactor.KindNPC}
	caster, _ := newAICaster(t, 1, target)

	var got EffectResult
	var calls int
	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects:     newEffectHandlers(effectsKnown{}, "DUMMYCAST", rec),
		Caster:      caster,
		OnHitResult: func(result EffectResult) {
			got = result
			calls++
		},
	}

	ai.Cast(target, ref)
	clock.advance(125 * time.Millisecond) // Launch
	clock.advance(400 * time.Millisecond) // Hit

	if calls != 1 {
		t.Fatalf("OnHitResult calls = %d, want 1", calls)
	}
	if got.AttackFailed != 1 {
		t.Fatalf("OnHitResult AttackFailed = %d, want 1", got.AttackFailed)
	}
}

// TestAIControllerCastStreamsHandlerMessagesToOnHitResult pins how NPC and
// summon casts deliver skill-handler messages: each message reaches
// OnHitResult on its own the moment the handler records it, ahead of the
// hit's later work on the target, and the final OnHitResult carries no
// messages. A summon MDAM that kills its target must report the owner's
// damage message before the target's death frames, as Mdam.java sends the
// damage message before reduceCurrentHp.
func TestAIControllerCastStreamsHandlerMessagesToOnHitResult(t *testing.T) {
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "MDAM"
	def.Offensive = true

	var log []any
	caster := &streamingSummonCaster{fakeCastCreature: fakeCastCreature{id: 1, kind: modelactor.KindSummon}, ownerID: 77}
	target := &mdamMarkerTarget{fakeCastCreature: fakeCastCreature{id: 2, kind: modelactor.KindNPC}, log: &log}

	var calls []EffectResult
	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects: EffectHandlers{
			Targets: skilltarget.NewRegistry(effectsKnown{}),
			Skills:  handlerskill.NewDefaultRegistry(),
		},
		Caster: caster,
		OnHitResult: func(result EffectResult) {
			calls = append(calls, result)
			log = append(log, result.Messages...)
		},
	}

	ai.Cast(target, ref)
	clock.advance(125 * time.Millisecond) // Launch
	clock.advance(400 * time.Millisecond) // Hit

	if len(calls) != 2 {
		t.Fatalf("OnHitResult calls = %d (%+v), want 2: the streamed damage message, then the final result", len(calls), calls)
	}
	if len(calls[0].Messages) != 1 {
		t.Fatalf("first OnHitResult Messages = %+v, want exactly the one damage message", calls[0].Messages)
	}
	damage, ok := calls[0].Messages[0].(handlerskill.Damage)
	if !ok || damage.RecipientID != 77 || damage.Source != handlerskill.DamageByServitor || damage.Amount <= 0 {
		t.Fatalf("streamed message = %#v, want the servitor Damage for owner 77", calls[0].Messages[0])
	}
	if got := calls[1].Messages; len(got) != 0 {
		t.Fatalf("final OnHitResult Messages = %+v, want none (already streamed)", got)
	}
	if len(log) != 2 || log[1] != mdamReduceHPMarker {
		t.Fatalf("delivery order = %+v, want [damage message, target ReduceHP]", log)
	}
}

type streamingSummonCaster struct {
	fakeCastCreature
	ownerID int32
}

func (c *streamingSummonCaster) OwnerID() int32 { return c.ownerID }

func (*streamingSummonCaster) IsPet() bool { return false }

const mdamReduceHPMarker = "target ReduceHP"

// mdamMarkerTarget takes a fixed positive magic hit and logs the moment its
// HP is reduced.
type mdamMarkerTarget struct {
	fakeCastCreature
	log *[]any
}

func (*mdamMarkerTarget) MagicDamageInput(creature.FormulaActor, modelskill.Definition, bool) (formulas.MagicDamageInput, bool) {
	return formulas.MagicDamageInput{MAtk: 100, MDef: 10, SkillPower: 10, PvPMul: 1, ElementalMul: 1, Shield: formulas.ShieldFailed}, true
}

func (m *mdamMarkerTarget) ReduceHP(float64, attackable.Combatant, modelskill.Definition) {
	*m.log = append(*m.log, mdamReduceHPMarker)
}

// TestAIControllerCastSkipsEffectsForFusionSkill matches
// CreatureCast.doFusionCast (CreatureCast.java:81-84), an empty stub for
// every non-player caster ("Non-Player Creatures cannot use FUSION or
// SIGNETS") — AIController drives exactly that non-player-initiated path, so
// a FUSION-skillType Hit must never reach the effect handlers, unlike a
// same-shaped non-FUSION skill.
func TestAIControllerCastSkipsEffectsForFusionSkill(t *testing.T) {
	clock := newCastClock()
	actor := scalingActor()
	ctrl := NewController(actor, nil)
	ctrl.SetQueue(clock.q)

	ref := modelskill.Ref{ID: scalingDef.ID, Level: scalingDef.Level}
	def := scalingDef
	def.Target = modelskill.TargetOne
	def.SkillType = "FUSION"

	rec := &recordingSkillHandler{}
	target := &fakeCastCreature{id: 2, kind: modelactor.KindNPC}
	caster, events := newAICaster(t, 1, target)

	ai := &AIController{
		Controller:  ctrl,
		Definitions: definitions(def),
		Effects:     newEffectHandlers(effectsKnown{}, "FUSION", rec),
		Caster:      caster,
	}

	ai.Cast(target, ref)

	// buildPlan skips atkSpd scaling for FUSION (controller.go:533-534,
	// matching PlayerCast.doFusionCast's raw getHitTime() read), so unlike
	// the atkSpd-scaled 125/400 choreography in the non-FUSION Hit test,
	// scalingDef's raw 1500ms HitTime yields LaunchDelay 1100ms, HitDelay
	// 400ms (controller.go:564-566).
	clock.advance(1100 * time.Millisecond)
	clock.advance(400 * time.Millisecond)

	if got := event.Count[event.SkillLaunched](events); got != 1 {
		t.Fatalf("SkillLaunched events = %d, want 1 (Hit phase must actually run for this test to prove anything)", got)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls after FUSION Hit phase = %d, want 0 (CreatureCast.doFusionCast is a no-op)", len(rec.calls))
	}
}

func TestAIControllerCastNoOpsForUnknownSkill(t *testing.T) {
	actor := &testActor{mp: 100, hp: 100}
	ctrl := NewController(actor, nil)
	ai := &AIController{Controller: ctrl, Definitions: definitions()}
	target := &fakeCastCreature{id: 2}

	ai.Cast(target, modelskill.Ref{ID: 999})

	if ctrl.CastingNow() {
		t.Fatal("CastingNow() = true after casting an unresolvable skill ref")
	}
}
