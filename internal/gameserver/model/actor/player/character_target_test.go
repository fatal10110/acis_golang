package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func targetCharacter(id int32) *Character {
	return &Character{ID: id, Name: "target", CharLevel: 1}
}

func TestCharacterTargetRoundTrips(t *testing.T) {
	c := targetCharacter(1)
	if got := c.Target(); got != nil {
		t.Fatalf("Target() = %v, want nil before any selection", got)
	}

	other := targetCharacter(2)
	c.SetTargetTracked(other)
	if got := c.Target(); got != world.Tracked(other) {
		t.Fatalf("Target() = %v, want %v", got, other)
	}

	c.SetTargetTracked(nil)
	if got := c.Target(); got != nil {
		t.Fatalf("Target() = %v, want nil after clearing", got)
	}
}

// TestCharacterRetargetableOnAggressionRetargetsWhenNotAlreadyTargetingCaster
// exercises the retargetableOnAggression contract the AGGDEBUFF continuous
// handler consults: a playable not currently targeting the caster gets
// retargeted onto them via SetTarget, not attacked.
func TestCharacterRetargetableOnAggressionRetargetsWhenNotAlreadyTargetingCaster(t *testing.T) {
	caster := targetCharacter(1)
	other := targetCharacter(3)
	target := targetCharacter(2)
	target.SetTargetTracked(other)

	var attacked bool
	target.SetAttackTargetHook(func(world.Tracked) { attacked = true })

	if got := target.CurrentTarget(); got != any(world.Tracked(other)) {
		t.Fatalf("CurrentTarget() = %v, want %v", got, other)
	}
	target.SetTarget(any(caster))

	if got := target.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() after SetTarget = %v, want caster", got)
	}
	if attacked {
		t.Fatal("a playable not already targeting the caster must be retargeted, not attacked")
	}
}

// TestCharacterRetargetableOnAggressionAttacksWhenAlreadyTargetingCaster
// exercises the other branch: a playable already targeting the caster is
// provoked into attacking them through the AttackTarget hook instead of
// being retargeted.
func TestCharacterRetargetableOnAggressionAttacksWhenAlreadyTargetingCaster(t *testing.T) {
	caster := targetCharacter(1)
	target := targetCharacter(2)
	target.SetTargetTracked(caster)

	var attackedWith any
	target.SetAttackTargetHook(func(t world.Tracked) { attackedWith = t })

	target.AttackTarget(any(caster))

	if attackedWith != any(world.Tracked(caster)) {
		t.Fatalf("AttackTarget hook called with %v, want caster", attackedWith)
	}
	if got := target.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() = %v, want unchanged caster (attack, not retarget)", got)
	}
}
