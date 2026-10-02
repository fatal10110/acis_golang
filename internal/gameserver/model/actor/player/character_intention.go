package player

import "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"

// IntentionSource reads a player's current intention from the network
// layer, which owns the attack, cast and follow intentions. Any goroutine
// may call it.
type IntentionSource interface {
	// IntentionFinalTarget is the object id of the creature the current
	// intention acts on (the cast's final target, the attack or follow
	// target), 0 when it acts on none.
	IntentionFinalTarget() int32
}

// SetIntentionSource wires the reader of c's current intention, called
// once by the network layer when it attaches c.
func (c *Character) SetIntentionSource(s IntentionSource) {
	c.intention.Store(&s)
}

// IntentionAimsAt reports whether c's current intention acts on t. A
// character with no intention source has none.
func (c *Character) IntentionAimsAt(t target.Actor) bool {
	p := c.intention.Load()
	if p == nil || *p == nil || t == nil {
		return false
	}
	id := (*p).IntentionFinalTarget()
	return id != 0 && id == t.ObjectID()
}
