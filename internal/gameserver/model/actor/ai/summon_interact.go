package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// positioned is a world object the AI can locate.
type positioned interface {
	Position() (x, y, z int)
}

// TryToInteract makes target, a world object that is no creature, the
// interact intention and evaluates it once; a summon mid-swing or mid-cast
// queues it instead. Taking the intention stops following whatever the
// summon followed. It reports false, changing nothing, when the summon cannot
// act.
func (s *Summon) TryToInteract(target world.Tracked, shift bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if target == nil || s.actor.DenyAIAction() {
		return false
	}
	in := intention{kind: IntentionInteract, object: target, shift: shift}
	if s.busyLocked() {
		s.next = in
		return true
	}
	s.setCurrentLocked(in)
	s.move.CancelFollow()
	s.thinkInteractLocked()
	return true
}

// thinkInteractLocked is one run of the interact intention. A summon never
// walks to such a target: a lost one turns following the owner back on and
// sends the summon idle; a shift-held one out of the summon's physical
// attack range (3D, no footprints) sends it idle; otherwise the summon keeps
// the intention where it stands.
func (s *Summon) thinkInteractLocked() {
	target := s.current.object
	at, ok := target.(positioned)
	if target == nil || !ok || !s.actor.KnowsObject(target) {
		s.actor.SetFollowOwner(true)
		s.goIdleLocked()
		return
	}
	if !s.current.shift {
		return
	}
	x, y, z := s.actor.Position()
	tx, ty, tz := at.Position()
	if !location.In3DRadius(x, y, z, tx, ty, tz, s.actor.PhysicalAttackRange()) {
		s.goIdleLocked()
	}
}
