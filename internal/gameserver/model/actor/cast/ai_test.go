package cast

import (
	"testing"
	"time"

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

func (f *fakeCastCreature) ObjectID() int32                { return f.id }
func (f *fakeCastCreature) Position() (int, int, int)      { return f.x, f.y, f.z }
func (f *fakeCastCreature) Heading() int                   { return 0 }
func (f *fakeCastCreature) Dead() bool                     { return f.dead }
func (f *fakeCastCreature) Category() skilltarget.Category { return f.category }
func (f *fakeCastCreature) SiegeGuard() bool               { return false }
func (f *fakeCastCreature) AlikeDead() bool                { return f.dead }

var _ attackable.Combatant = (*fakeCastCreature)(nil)
var _ skilltarget.Creature = (*fakeCastCreature)(nil)
var _ Target = (*fakeCastCreature)(nil)
