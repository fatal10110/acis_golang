package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// BoatEntrance routes a summon's friendly follow of anyone but its owner
// through the entrance of a boat's dock, as SummonMove.friendlyFollowTask
// does through Playable.tryToPassBoatEntrance.
type BoatEntrance interface {
	// PassBoatEntrance returns the shore point where the summon's walk
	// toward dest crosses the entrance of the dock a boat it knows serves,
	// and true when that point lies beyond the entrance reach: the summon
	// then walks there. Otherwise (no boat known, no entrance crossed, or the
	// summon at the entrance already) it answers the owner ActionFailed and
	// returns false.
	PassBoatEntrance(dest location.Location) (location.Location, bool)
	// Refuse answers the owner ActionFailed.
	Refuse()
}

// movementDisabler is a summon that can be rooted, stunned, asleep, ...
type movementDisabler interface {
	MovementDisabled() bool
}

// SetBoatEntrance wires the summon's friendly follow of anyone but its owner
// to boats. Left unset (the default), such a follow walks after its target
// like an owner follow.
func (s *Summon) SetBoatEntrance(boats BoatEntrance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.boats = boats
}

// followThroughBoatEntranceLocked is one friendly follow run toward target
// when target is not the summon's owner nor the owner's own
// (SummonMove.java:94-99): the follow never walks after target. Out of reach,
// the summon heads for the boat entrance its walk toward target crosses,
// that walk replacing the follow (PlayableAI.tryToMoveTo); with no entrance
// to head for, the owner is answered ActionFailed and the summon stays. It
// reports whether it handled the run.
func (s *Summon) followThroughBoatEntranceLocked(target attackable.Combatant) (bool, error) {
	if s.boats == nil {
		return false, nil
	}
	if owner, ok := s.actor.Owner(); ok && owner != nil && sameCombatant(actingPlayer(target), owner) {
		return false, nil
	}
	dest, outOfReach := s.move.MaybeStartEntranceFollow(target, summonFollowOffset)
	if !outOfReach {
		return true, nil
	}
	entrance, walk := s.boats.PassBoatEntrance(dest)
	if !walk {
		return true, nil
	}
	s.setCurrentLocked(intention{kind: IntentionMoveTo, loc: entrance})
	s.move.CancelFollow()
	// PlayableAI.thinkMoveTo: a summon that cannot move goes idle, answered
	// ActionFailed.
	if disabler, ok := s.actor.(movementDisabler); ok && disabler.MovementDisabled() {
		s.idleLocked()
		s.boats.Refuse()
		return true, nil
	}
	_, err := s.move.MoveToLocation(entrance)
	return true, err
}
