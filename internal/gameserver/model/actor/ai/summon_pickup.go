package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// SummonPickUp is the ground-item side of a pet's pick-up intention. Its
// refusals reach the pet's owner.
type SummonPickUp interface {
	// PickUpTarget returns where the ground item itemObjectID lies.
	// onGround is false once it has left the world or the ground, seen
	// false while it lies outside the summon's sight.
	PickUpTarget(itemObjectID int32) (at location.Location, onGround, seen bool)
	// Idle sends the summon idle the way its owner's Stop command does:
	// one that follows its owner goes back to following it.
	Idle()
	// FollowOwner turns following the owner back on, sending the summon
	// back to it.
	FollowOwner()
	// PickUp has the pet take the ground item itemObjectID, telling the
	// owner why when it cannot.
	PickUp(itemObjectID int32)
}

// SetPickUp wires the summon's pick-up intention to the ground items. Left
// unset (the default), TryToPickUp refuses every request.
func (s *Summon) SetPickUp(pickup SummonPickUp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pickup = pickup
}

// TryToPickUp makes picking up the ground item itemObjectID the current
// intention and acts on it once; a summon mid-swing or mid-cast queues it
// instead. It reports false, changing nothing, when the summon cannot act or
// has no pick-up wiring.
func (s *Summon) TryToPickUp(itemObjectID int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pickup == nil || s.actor.DenyAIAction() {
		return false
	}
	in := intention{kind: IntentionPickUp, itemObjectID: itemObjectID}
	if s.busyLocked() {
		s.next = in
		return true
	}
	s.setCurrentLocked(in)
	if err := s.thinkPickUpLocked(); err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return true
}

// thinkPickUpLocked is one run of the pick-up intention. A summon that
// cannot act, or whose item has left the ground, goes idle; one that has
// lost sight of the item goes back to following its owner. One not yet
// within PickUpRange of the item walks straight to it, path-finding off,
// unless it cannot move or that walk is already under way; the intention
// holds either way, so the walk's end (or the next think) runs it again.
// Within range the summon goes idle, then takes the item.
func (s *Summon) thinkPickUpLocked() error {
	if s.pickup == nil {
		s.current = intention{kind: IntentionIdle}
		return nil
	}
	id := s.current.itemObjectID
	if s.actor.DenyAIAction() {
		s.endPickUpLocked(SummonPickUp.Idle)
		return nil
	}
	at, onGround, seen := s.pickup.PickUpTarget(id)
	if !onGround {
		s.endPickUpLocked(SummonPickUp.Idle)
		return nil
	}
	if !seen {
		s.endPickUpLocked(SummonPickUp.FollowOwner)
		return nil
	}
	x, y, z := s.actor.Position()
	if (location.Location{X: x, Y: y, Z: z}).Distance3D(at) >= PickUpRange {
		// A periodic think must not restart the walk, which would
		// broadcast it again.
		if s.current.walking && s.actor.IsMoving() {
			return nil
		}
		if d, ok := s.actor.(movementDisabler); ok && d.MovementDisabled() {
			return nil
		}
		// The follow the walk replaces must not pull the summon back.
		s.move.CancelFollow()
		walking, err := s.move.MoveStraightToLocation(at)
		s.current.walking = walking
		return err
	}
	s.endPickUpLocked(func(p SummonPickUp) {
		p.Idle()
		p.PickUp(id)
	})
	return nil
}

// endPickUpLocked ends the pick-up intention, then runs then on the
// pick-up wiring with mu released, as it reaches back into the summon's AI
// and its owner; the caller holds mu and holds it again on return, a panic
// included.
func (s *Summon) endPickUpLocked(then func(SummonPickUp)) {
	s.current = intention{kind: IntentionIdle}
	pickup := s.pickup
	s.mu.Unlock()
	defer s.mu.Lock()
	then(pickup)
}
