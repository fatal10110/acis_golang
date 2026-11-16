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

// SpawnBesideOwner is SpawnAt at owner plus offset, facing the way owner
// faces.
func SpawnBesideOwner(state *world.State, actor *Actor, owner Owner, offset location.Location) {
	if owner == nil {
		return
	}
	x, y, z := owner.Position()
	SpawnAt(state, actor, owner, location.Location{X: x + offset.X, Y: y + offset.Y, Z: z + offset.Z}, owner.Heading())
}

// SpawnAt places actor in state at at, facing heading, and registers it as
// owner's active summon. A living summon starts its lifetime task (a
// servitor's countdown, a pet's feeding), and a living baby pet starts
// healing its owner.
func SpawnAt(state *world.State, actor *Actor, owner Owner, at location.Location, heading int) {
	if state == nil || actor == nil || owner == nil {
		return
	}
	actor.bindOwner(owner, actor.ownerInv())
	actor.world = state
	state.Spawn(actor, at.X, at.Y, at.Z, heading)
	state.AddSummon(owner.ObjectID(), actor)
	actor.EnterZones()
	// Vitals restored short regenerate one period from the spawn.
	actor.SettleRegen()
	actor.startLifeTask()
	actor.startBabyHeal()
}
