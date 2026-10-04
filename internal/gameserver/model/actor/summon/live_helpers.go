package summon

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func sameObject(a, b world.Tracked) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ObjectID() == b.ObjectID()
}

func (a *Actor) ownerWithinFollowRange() bool {
	owner := a.currentOwner()
	if owner == nil {
		return false
	}
	ax, ay, az := a.Position()
	bx, by, bz := owner.Position()
	return location.In3DRadius(ax, ay, az, bx, by, bz, 2000)
}

func feedbackFor(outcome Outcome) Feedback {
	switch outcome {
	case OutcomeRefusedOutOfControl:
		return FeedbackPetRefusingOrder
	case OutcomeRefusedDead:
		return FeedbackDeadPetCannotBeReturned
	case OutcomeRefusedInCombat:
		return FeedbackPetCannotBeSentBackDuringBattle
	case OutcomeRefusedHungry:
		return FeedbackCannotRestoreHungryPet
	case OutcomeRefusedLevelGap:
		return FeedbackPetTooHighToControl
	default:
		return FeedbackNone
	}
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultRoll(roll func(int) int) func(int) int {
	if roll != nil {
		return roll
	}
	return rand.IntN
}

// SpawnBesideOwner places actor in state at owner plus offset and registers
// it as the owner's active summon. A living baby pet starts healing its
// owner.
func SpawnBesideOwner(state *world.State, actor *Actor, owner Owner, offset location.Location) {
	if state == nil || actor == nil || owner == nil {
		return
	}
	actor.bindOwner(owner, actor.ownerInv())
	actor.world = state
	x, y, z := owner.Position()
	state.Spawn(actor, x+offset.X, y+offset.Y, z+offset.Z, owner.Heading())
	state.AddSummon(owner.ObjectID(), actor)
	actor.EnterZones()
	actor.startBabyHeal()
}
