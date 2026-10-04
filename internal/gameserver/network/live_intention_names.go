package network

// intentionNames names p's current and next intentions as a game master's
// character page shows them, IDLE for none. The current one is read from
// the slot that holds it: the cast in flight or its approach, the attack,
// a pickup, an interact walk, a friendly follow, a walk to a point or an
// equip toggle. The next one is whatever is queued behind it.
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
		}
	}

	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	switch {
	case p.deferredPickup != nil:
		return current, "PICK_UP"
	case p.deferredMagic != nil && p.deferredMagic.approach == 0, p.deferredItem != nil && p.deferredItem.approach == 0:
		// A cast still walking to its target is the current intention.
		return current, "CAST"
	case p.deferredFollow != nil:
		return current, "FOLLOW"
	case p.deferredUseItem != nil:
		return current, "USE_ITEM"
	case p.deferredInteract != nil:
		return current, "INTERACT"
	}
	return current, "IDLE"
}
