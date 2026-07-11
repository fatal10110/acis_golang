// Package ai models actor intention loops.
package ai

// Intention identifies the current action a creature AI is trying to run.
type Intention uint8

const (
	// IntentionIdle means the actor has no active desire.
	IntentionIdle Intention = iota
	// IntentionAttack means the actor is trying to attack a target.
	IntentionAttack
	// IntentionWander means the actor is walking around its spawn territory.
	IntentionWander
)
