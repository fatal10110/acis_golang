package network

// IntentionFinalTarget is the object id of the creature p's current
// intention acts on, 0 when it acts on none: the cast in flight's target,
// the final target a cast approach walks to, the attack target, or the
// player a friendly follow follows. A walk, pickup, interact, posture
// change or idle acts on none. Any goroutine may call it: a summon's cast
// policy reads it from the summon's queue.
func (p *livePlayer) IntentionFinalTarget() int32 {
	if p.cast != nil {
		if _, target, ok := p.cast.InFlight(); ok {
			if target == nil {
				return 0
			}
			return target.ObjectID()
		}
	}
	if id := p.castApproachTarget(); id != 0 {
		return id
	}
	if p.combat != nil {
		if target := p.combat.Target(); target != nil {
			return target.ObjectID()
		}
	}
	if p.move != nil {
		if target := p.move.FriendlyFollowTarget(); target != nil {
			return target.ObjectID()
		}
	}
	return 0
}

// castApproachTarget is the final target id of the cast approach that is
// p's current CAST intention, 0 when none walks.
func (p *livePlayer) castApproachTarget() int32 {
	p.pickupMu.Lock()
	defer p.pickupMu.Unlock()
	switch {
	case p.deferredMagic != nil:
		return p.deferredMagic.approach
	case p.deferredItem != nil:
		return p.deferredItem.approach
	}
	return 0
}
