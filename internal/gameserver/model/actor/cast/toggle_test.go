package cast

import (
	"errors"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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

func TestCanCastToggleOnlyChecksReuseDelay(t *testing.T) {
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
