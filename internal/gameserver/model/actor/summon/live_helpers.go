package summon

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func sameObject(a, b worldobject.Object) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ObjectID() == b.ObjectID()
}

func (a *Actor) ownerWithinFollowRange() bool {
	if a.owner == nil {
		return false
	}
	ax, ay, az := a.Position()
	bx, by, bz := a.owner.Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, 2000)
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
// it as the owner's active summon.
func SpawnBesideOwner(state *world.State, actor *Actor, owner Owner, offset location.Location) {
	if state == nil || actor == nil || owner == nil {
		return
	}
	actor.owner = owner
	actor.world = state
	x, y, z := owner.Position()
	state.Spawn(actor, x+offset.X, y+offset.Y, z+offset.Z, ownerHeading(owner))
	state.AddSummon(owner.ObjectID(), actor)
}

func ownerHeading(owner Owner) int {
	h, ok := owner.(interface{ Heading() int })
	if !ok {
		return 0
	}
	return h.Heading()
}
