package cast

import (
	"errors"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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
	if err := NewController(actor, nil).CanCastToggle(def); err != nil {
		t.Fatalf("CanCastToggle() error = %v, want nil despite empty resources", err)
	}

	actor.disabledKeys = map[int32]bool{ReuseKey(def): true}
	if err := NewController(actor, nil).CanCastToggle(def); !errors.Is(err, ErrSkillDisabled) {
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
	if err := NewController(actor, nil).CanCastToggle(def); !errors.Is(err, ErrAllSkillsDisabled) {
		t.Fatalf("CanCastToggle() error = %v, want ErrAllSkillsDisabled", err)
	}
}

// groundTestActor adds an optional GroundTarget surface on top of testActor,
// matching the reference's Player-only _signetLocation (PlayerCast.java:42):
// a non-player caster (plain *testActor) never implements this interface,
// so the unset-signet gate only ever applies to a caster that can carry one.
type groundTestActor struct {
	*testActor
	gx, gy, gz int
}

func (a *groundTestActor) GroundTargetUnset() bool { return a.gx == 0 && a.gy == 0 && a.gz == 0 }

// TestCanAttemptCastRejectsUnsetGroundTarget covers PlayerCast.canAttemptCast
// (PlayerCast.java:224): a GROUND skill requested before any
// RequestExMagicSkillUseGround recorded a signet point is rejected —
// Location.DUMMY_LOC is (0,0,0), matching the zero-value GroundTarget here.
func TestCanAttemptCastRejectsUnsetGroundTarget(t *testing.T) {
	def := modelskill.Definition{ID: 5, Level: 1, Target: modelskill.TargetGround}
	actor := &groundTestActor{testActor: &testActor{}}
	if err := NewController(actor, nil).CanAttemptCast(testTarget{}, def); !errors.Is(err, ErrGroundTargetUnset) {
		t.Fatalf("CanAttemptCast() error = %v, want ErrGroundTargetUnset", err)
	}

	actor.gx, actor.gy, actor.gz = 1000, 2000, 300
	if err := NewController(actor, nil).CanAttemptCast(testTarget{}, def); err != nil {
		t.Fatalf("CanAttemptCast() error = %v, want nil once the signet point is set", err)
	}
}

// TestCanAttemptCastSkipsGroundGateForNonGroundTargeter covers a caster type
// that cannot carry a signet point at all (a plain *testActor, standing in
// for a non-player creature): the gate must not panic or misfire on a type
// assertion failure, it simply doesn't apply.
func TestCanAttemptCastSkipsGroundGateForNonGroundTargeter(t *testing.T) {
	def := modelskill.Definition{ID: 5, Level: 1, Target: modelskill.TargetGround}
	actor := &testActor{}
	if err := NewController(actor, nil).CanAttemptCast(testTarget{}, def); err != nil {
		t.Fatalf("CanAttemptCast() error = %v, want nil for a caster with no GroundTarget surface", err)
	}
}

func TestCanCastToggleRejectsNonToggleSkill(t *testing.T) {
	def := toggleDef()
	def.Activation = modelskill.ActivationActive

	actor := &testActor{mp: 100, hp: 100}
	if err := NewController(actor, nil).CanCastToggle(def); err == nil {
		t.Fatal("CanCastToggle() error = nil, want an error for a non-toggle skill")
	}
}

func TestCastToggleDeactivatesAnAlreadyActiveInstanceAtNoCost(t *testing.T) {
	actor := &testActor{mp: 5, hp: 5}
	def := toggleDef()
	def.HPConsume = 3

	activated, err := NewController(actor, nil).CastToggle(true, def)
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

	activated, err := NewController(actor, nil).CastToggle(false, def)
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

		if _, err := NewController(actor, nil).CastToggle(false, def); !errors.Is(err, ErrNotEnoughMP) {
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

		if _, err := NewController(actor, nil).CastToggle(false, def); !errors.Is(err, ErrNotEnoughHP) {
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

	if _, err := NewController(actor, nil).CastToggle(false, def); err != nil {
		t.Fatalf("CastToggle() error: %v", err)
	}
	if len(actor.disabled) != 0 || len(actor.reuses) != 0 {
		t.Fatalf("cooldown state after activate = disabled %+v reuses %+v, want none", actor.disabled, actor.reuses)
	}
}
