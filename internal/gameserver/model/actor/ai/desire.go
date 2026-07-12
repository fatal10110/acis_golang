package ai

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

// Desire is a weighted request for one action an actor's AI might take
// next: an Intention kind plus every parameter that action needs, a weight
// used to rank it against every other pending Desire in a DesireQueue, and
// the time it was queued.
//
// A single actor typically holds several Desires at once (e.g. one ATTACK
// Desire per creature it is threatened by); DesireQueue.Peek resolves that
// set down to the one Desire with the highest weight.
type Desire struct {
	Kind Intention

	// Target is the non-creature object this Desire acts on, e.g. an item
	// to pick up or an object to interact with.
	Target worldobject.Object
	// FinalTarget is the creature this Desire acts on: the attack, cast,
	// flee or follow target.
	FinalTarget attackable.Combatant

	Skill    skill.Ref
	Location location.Location

	CtrlPressed  bool
	ShiftPressed bool

	ItemObjectID int32
	RouteName    string
	Timer        int
	MoveToTarget bool

	Weight   float64
	QueuedAt time.Time
}

// Equal reports whether d and other request the same action for queueing
// purposes: matching Kind, and — for kinds where more than one live
// instance makes sense — a matching kind-specific parameter:
//
//   - Idle, Nothing and Wander carry no parameters, so any two instances of
//     the same kind are equal.
//   - Attack, Flee and Follow are equal when they share the same
//     FinalTarget.
//   - Cast is equal when it shares both FinalTarget and Skill.
//   - PickUp and Social are equal when they share the same ItemObjectID.
//   - MoveRoute is equal when it names the same RouteName.
//   - MoveTo is equal when the two Locations are within 20 units on the
//     ground and 30 units in height of each other.
//   - Every other kind (FakeDeath, Interact, Sit, Stand, UseItem) has no
//     natural dedupe key and is never equal to another instance of itself,
//     so each request keeps its own queue slot.
func (d *Desire) Equal(other *Desire) bool {
	if d == other {
		return true
	}
	if other == nil || d.Kind != other.Kind {
		return false
	}

	switch d.Kind {
	case IntentionIdle, IntentionNothing, IntentionWander:
		return true
	case IntentionAttack, IntentionFlee, IntentionFollow:
		return sameCombatant(d.FinalTarget, other.FinalTarget)
	case IntentionCast:
		return sameCombatant(d.FinalTarget, other.FinalTarget) && d.Skill == other.Skill
	case IntentionPickUp, IntentionSocial:
		return d.ItemObjectID == other.ItemObjectID
	case IntentionMoveRoute:
		return d.RouteName == other.RouteName
	case IntentionMoveTo:
		return d.Location.Distance2D(other.Location) <= 20 && absInt(d.Location.Z-other.Location.Z) <= 30
	default:
		return false
	}
}

// addWeight raises d's weight by delta, capped at the largest representable
// float64 so repeated merges can never overflow to infinity.
func (d *Desire) addWeight(delta float64) {
	d.Weight = math.Min(d.Weight+delta, math.MaxFloat64)
}

func sameCombatant(a, b attackable.Combatant) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.ObjectID() == b.ObjectID()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
