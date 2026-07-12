// Package ai models actor intention loops.
package ai

// Intention identifies the kind of action a creature AI is trying to run.
type Intention uint8

const (
	// IntentionIdle means the actor has no active desire.
	IntentionIdle Intention = iota
	// IntentionAttack means the actor is trying to attack a target.
	IntentionAttack
	// IntentionCast means the actor is trying to cast a skill on a target.
	IntentionCast
	// IntentionFakeDeath means the actor is feigning death.
	IntentionFakeDeath
	// IntentionFlee means the actor is running away from a target.
	IntentionFlee
	// IntentionFollow means the actor is tracking and following a target's
	// movement.
	IntentionFollow
	// IntentionInteract means the actor is moving to and interacting with
	// an object.
	IntentionInteract
	// IntentionMoveRoute means the actor is walking a named waypoint route.
	IntentionMoveRoute
	// IntentionMoveTo means the actor is moving to a fixed location.
	IntentionMoveTo
	// IntentionNothing means the actor is deliberately doing nothing, used
	// as a low-priority filler desire.
	IntentionNothing
	// IntentionPickUp means the actor is moving to and picking up an item.
	IntentionPickUp
	// IntentionSit means the actor is sitting down.
	IntentionSit
	// IntentionSocial means the actor is playing a social action.
	IntentionSocial
	// IntentionStand means the actor is standing up.
	IntentionStand
	// IntentionUseItem means the actor is using an item.
	IntentionUseItem
	// IntentionWander means the actor is walking around its spawn
	// territory.
	IntentionWander
)

// String returns a short lowercase name for the Intention, used in logs and
// test failure messages.
func (i Intention) String() string {
	switch i {
	case IntentionIdle:
		return "idle"
	case IntentionAttack:
		return "attack"
	case IntentionCast:
		return "cast"
	case IntentionFakeDeath:
		return "fake_death"
	case IntentionFlee:
		return "flee"
	case IntentionFollow:
		return "follow"
	case IntentionInteract:
		return "interact"
	case IntentionMoveRoute:
		return "move_route"
	case IntentionMoveTo:
		return "move_to"
	case IntentionNothing:
		return "nothing"
	case IntentionPickUp:
		return "pick_up"
	case IntentionSit:
		return "sit"
	case IntentionSocial:
		return "social"
	case IntentionStand:
		return "stand"
	case IntentionUseItem:
		return "use_item"
	case IntentionWander:
		return "wander"
	default:
		return "unknown"
	}
}
