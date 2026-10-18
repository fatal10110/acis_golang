package network

// intentionNames names p's current and next intentions as a game master's
// character page shows them, IDLE for none. The current one is read from
// the slot that holds it: the cast in flight or its approach, the attack,
// a pickup, an interact walk, a friendly follow, a walk to a point, an
// equip toggle, or a sit or stand request under way. The next one is
// whatever is queued behind it: a queued pickup, cast, follow, equip
// toggle, interact or other request held behind a swing, cast or posture
// change runs ahead of a queued attack when the swing or cast ends, so it
// is named first.
func (p *livePlayer) intentionNames() (current, next string) {
	current = "IDLE"
	switch {
	case p.cast != nil && p.cast.CastingNow(), p.castApproachTarget() != 0:
		current = "CAST"
	case p.combat != nil && p.combat.Target() != nil:
		current = "ATTACK"
	case p.hasPickup():
		current = "PICK_UP"
	case p.hasInteract():
		current = "INTERACT"
	case p.move != nil && p.move.FriendlyFollowTarget() != nil:
		current = "FOLLOW"
	default:
		switch p.heldIntention().kind {
		case heldMoveTo:
			current = "MOVE_TO"
		case heldUseItem:
			current = "USE_ITEM"
		case heldSit:
			current = "SIT"
		case heldStand:
			current = "STAND"
		}
	}

	if next = p.queuedIntentionName(); next == "IDLE" && p.combat != nil && p.combat.HasNext() {
		next = "ATTACK"
	}
	return current, next
}

// queuedIntentionName names the intention queued in p's own next-intention
// slots, IDLE for none.
func (p *livePlayer) queuedIntentionName() string {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	switch {
	case p.deferredPickup != nil:
		return "PICK_UP"
	case p.deferredMagic != nil && p.deferredMagic.approach == 0, p.deferredItem != nil && p.deferredItem.approach == 0:
		// A cast still walking to its target is the current intention.
		return "CAST"
	case p.deferredFollow != nil:
		return "FOLLOW"
	case p.deferredUseItem != nil:
		return "USE_ITEM"
	case p.deferredInteract != nil:
		return "INTERACT"
	case p.deferredAction != nil:
		return p.deferredActionIntention
	}
	return "IDLE"
}
