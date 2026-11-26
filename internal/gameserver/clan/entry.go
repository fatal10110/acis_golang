package clan

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// entries tracks the characters whose selection is in flight, from before
// their row is read until they enter the world or the selection ends. Such
// a character is offline to its clan, so a removal in that span stores the
// leave on its row only; the removal is kept here for the character the
// selection already loaded. It also keeps the removals handed to members
// leaving the world (handOffLocked) until they are marked offline. mu may
// be taken while a clan's mu is held, never the other way round.
type entries struct {
	mu sync.Mutex
	// departed maps each character in flight to the removal it took, nil
	// while it took none.
	departed map[int32]*RemovalRow
	// leaving maps each member removed on its way out of the world to the
	// removal handed to it.
	leaving map[int32]handOff
}

// begin puts objectID in flight, forgetting any removal kept before.
func (e *entries) begin(objectID int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.departed == nil {
		e.departed = make(map[int32]*RemovalRow)
	}
	e.departed[objectID] = nil
}

// depart keeps row for its character when that one is in flight,
// reporting whether it was.
func (e *entries) depart(row RemovalRow) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.departed[row.ObjectID]; !ok {
		return false
	}
	e.departed[row.ObjectID] = &row
	return true
}

// take returns the removal kept for objectID and forgets it; end also takes
// objectID out of flight.
func (e *entries) take(objectID int32, end bool) (RemovalRow, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	row, ok := e.departed[objectID]
	switch {
	case end:
		delete(e.departed, objectID)
	case ok:
		e.departed[objectID] = nil
	}
	if row == nil {
		return RemovalRow{}, false
	}
	return *row, true
}

// BeginEntry marks objectID's selection in flight, ahead of reading its row:
// a removal from its clan from now on reaches the character the selection
// loads, at RestoreMembership or Enter.
func (s *Service) BeginEntry(objectID int32) {
	s.entries.begin(objectID)
}

// EndEntry ends c's selection without it entering the world: the selection
// failed, or c leaves before it entered. c takes a removal kept for it, so a
// last save stores what the removal did.
func (s *Service) EndEntry(c *player.Character) {
	if row, ok := s.entries.take(c.ID, true); ok {
		s.applyDeparture(c, row)
	}
}

// Enter settles c's clan state as it enters the world, on its queue, and
// ends its selection. Still a member, c is marked online in its clan, which
// is returned; removed while its selection was in flight, c takes the leave
// its removal stored, as a fresh load of its row would show it.
func (s *Service) Enter(c *player.Character) (*Clan, bool) {
	if cl, ok := s.table.Get(c.ClanID()); ok && cl.SetOnline(c.ID, LiveMember(c)) {
		s.entries.take(c.ID, true)
		return cl, true
	}
	if row, ok := s.entries.take(c.ID, true); ok {
		s.applyDeparture(c, row)
	} else {
		s.leaveStaleClan(c)
	}
	return nil, false
}

// applyDeparture gives c, loaded before its clan removed it, the clan
// columns the removal stored: no clan and no title, the stored penalties,
// no peace declared; its siege state clears and its pledge class follows.
func (s *Service) applyDeparture(c *player.Character, row RemovalRow) {
	c.SetTitle("")
	c.SetClanID(0)
	c.SetSiegeState(player.SiegeStateNone)
	c.SetClanJoinExpiryTime(row.JoinExpiry)
	c.SetClanCreateExpiryTime(row.CreateExpiry)
	c.SetWantsPeace(false)
	c.SetPledgeClass(s.pledgeClass(c))
}
