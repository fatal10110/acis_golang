package clan

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// handOff is a removal kept for a member that was leaving the world, its
// row still to be written: the member removed, whether it led its clan,
// the join penalty's end and when it was removed.
type handOff struct {
	member     Member
	leader     bool
	joinExpiry int64
	now        time.Time
}

// handOff keeps h for its member.
func (e *entries) handOff(h handOff) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.leaving == nil {
		e.leaving = make(map[int32]handOff)
	}
	e.leaving[h.member.ObjectID] = h
}

// takeHandOff returns the removal handed to objectID and forgets it.
func (e *entries) takeHandOff(objectID int32) (handOff, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	h, ok := e.leaving[objectID]
	delete(e.leaving, objectID)
	return h, ok
}

// Leave marks c, out of the world on its way out, offline in its clan,
// which is returned; it runs on c's queue. Removed from its clan since it
// left the world, c instead takes the removal handed to it: its row is
// written as an online member's, behind the saves c queued on its way out,
// which took it online, and c takes the leave that row stores.
func (s *Service) Leave(c *player.Character) (*Clan, bool) {
	if cl, ok := s.table.Get(c.ClanID()); ok && cl.SetOffline(c.ID, LiveMember(c)) {
		return cl, true
	}
	h, ok := s.entries.takeHandOff(c.ID)
	if !ok {
		return nil, false
	}
	row := s.removalRow(h.member, h.leader, h.joinExpiry, c, h.now)
	s.write(c.ID, "remove clan member", func(ctx context.Context, st Store) error { return st.RemoveMembership(ctx, row) })
	s.applyDeparture(c, row)
	return nil, false
}
