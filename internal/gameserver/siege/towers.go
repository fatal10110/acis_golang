package siege

import "github.com/fatal10110/acis_golang/internal/gameserver/castle"

// Tower is one of the castle's control towers.
type Tower = castle.Tower

// towersLocked has step taken by each of the castle's control towers, in
// castles.xml order, once the engine's lock is released.
func (s *Siege) towersLocked(fx *effects, step func(Tower)) {
	towers := s.castle.Towers()
	if len(towers) == 0 {
		return
	}
	fx.add(func() {
		for _, t := range towers {
			step(t)
		}
	})
}

// AliveLifeTowers counts the castle's life control towers standing: every
// one outside the siege, the working ones while it is fought.
func (s *Siege) AliveLifeTowers() int { return s.castle.AliveLifeTowers(s.InProgress()) }

// LifeTowerDown is one of the castle's life control towers falling dead
// while the siege is fought: once none stands, the owner and the defenders
// are told their dead can no longer be resurrected.
func (s *Siege) LifeTowerDown() {
	if s.AliveLifeTowers() == 0 {
		s.Tell(Message{ID: MsgTowerDestroyedNoResurrection}, SideDefender)
	}
}
