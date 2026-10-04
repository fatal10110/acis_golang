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

// queueCastIntention records a CAST intention just queued as p's next
// intention. Behind a swing in flight, the attack the swing is for stays
// current, its final target still p's main target, until the swing ends and
// the queued cast replaces it (replaceAttackWithQueuedCast); only an attack
// queued as the next intention is replaced now. Behind a cast or a posture
// change, or a pets-row read standing in for a cast, no attack is current:
// the cast replaces whatever attack waits.
func (p *livePlayer) queueCastIntention() {
	if p.combat == nil {
		return
	}
	if p.attack != nil && p.attack.AttackingNow() {
		p.combat.QueueCastBehindSwing()
		return
	}
	p.combat.ReplaceWithCast()
}

// replaceAttackWithQueuedCast drops the attack a swing or bow shot that just
// ended was for when a cast is queued behind it: the cast is the intention
// now, whether it starts, is refused, or waits on a posture change.
func (p *livePlayer) replaceAttackWithQueuedCast() {
	if p.combat != nil && p.hasParkedCast() {
		p.combat.ReplaceWithCast()
	}
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
