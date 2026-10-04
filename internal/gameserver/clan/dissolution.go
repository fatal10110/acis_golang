package clan

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// minDissolveDelay is the least a scheduled dissolution waits: a clan
// restored with its dissolution already past due is destroyed a minute
// after boot.
const minDissolveDelay = time.Minute

// DissolveResult is the outcome of a clan leader's dissolution request.
type DissolveResult int

// The dissolution outcomes, the refusals in the order they are checked.
const (
	// DissolveScheduled: the clan is destroyed once DissolveDays pass,
	// unless its leader recovers it first.
	DissolveScheduled DissolveResult = iota
	// DissolveNow: DissolveDays is 0, so the caller destroys the clan at
	// once (Destroy).
	DissolveNow
	DissolveNotLeader
	DissolveInAlliance
	DissolveAtWar
	DissolveOwnsResidence
	DissolveInProgress
)

// RequestDissolve has c, its clan's leader, ask for the clan to be
// dissolved as of now. A clan in an alliance, at war, or owning a castle or
// clan hall may not be dissolved, nor one whose dissolution is already
// pending. A clan registered on a castle siege may not be either, which
// is not checked yet (#3330).
func (s *Service) RequestDissolve(c *player.Character, now time.Time) (*Clan, DissolveResult) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return nil, DissolveNotLeader
	}
	nowMs := now.UnixMilli()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	switch {
	case cl.destroyed:
		return nil, DissolveNotLeader
	case cl.allyID != 0:
		return cl, DissolveInAlliance
	case len(cl.wars) > 0:
		return cl, DissolveAtWar
	case cl.castleID > 0 || cl.hallID > 0:
		return cl, DissolveOwnsResidence
	case cl.dissolvingExpiry > nowMs:
		return cl, DissolveInProgress
	}
	if s.cfg.DissolveDays <= 0 {
		return cl, DissolveNow
	}
	cl.dissolvingExpiry = nowMs + int64(s.cfg.DissolveDays)*dayMillis
	s.updateClanLocked(cl)
	s.dissolutions.schedule(cl, time.Duration(cl.dissolvingExpiry-nowMs)*time.Millisecond)
	return cl, DissolveScheduled
}

// RecoverResult is the outcome of a clan leader's recovery request.
type RecoverResult int

// The recovery outcomes, the refusals in the order they are checked.
const (
	Recovered RecoverResult = iota
	RecoverNotLeader
	// RecoverNothing refuses a clan with no dissolution pending.
	RecoverNothing
)

// RecoverClan has c, its clan's leader, call off the clan's pending
// dissolution.
func (s *Service) RecoverClan(c *player.Character) (*Clan, RecoverResult) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return nil, RecoverNotLeader
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	switch {
	case cl.destroyed:
		return nil, RecoverNotLeader
	case cl.dissolvingExpiry <= 0:
		return cl, RecoverNothing
	}
	cl.dissolvingExpiry = 0
	s.updateClanLocked(cl)
	s.dissolutions.cancel(cl.id)
	return cl, Recovered
}

// Dissolution is what a destroyed clan lost: its leader and its roster,
// each member as it stood when it was removed.
type Dissolution struct {
	LeaderID int32
	Members  []Member
}

// Destroy dissolves cl as of now, reporting false when it is already gone,
// or, when due is set (a scheduled dissolution coming due), when its
// dissolution was called off since. The clan leaves the registry; every
// clan at war with it, either way, forgets the war; every member leaves it
// as a member who withdrew does, without a join penalty, its leader with
// the creation penalty a leader leaving takes; then its rows go: the clan,
// its privileges, skills, sub-units, wars and siege registrations, and the
// tax of a castle it held is reset. online resolves a member's live
// character, nil when it is not in the world. The members' live state is
// the caller's to clear (ApplyDispersed), as is the clan warehouse.
// Dropping the clan from the sieges' registrations and the siegable halls
// is not ported yet (#3330).
func (s *Service) Destroy(cl *Clan, due bool, online func(int32) *player.Character, now time.Time) (Dissolution, bool) {
	cl.mu.Lock()
	if cl.destroyed || (due && cl.dissolvingExpiry == 0) {
		cl.mu.Unlock()
		return Dissolution{}, false
	}
	// Set under mu, so a war declared on cl from here on is refused and
	// one declared before is in its lists below.
	cl.destroyed = true
	leaderID, castleID := cl.leaderID, cl.castleID
	cl.mu.Unlock()
	s.table.remove(cl.id)
	s.dissolutions.cancel(cl.id)
	s.forgetWars(cl)

	out := Dissolution{LeaderID: leaderID}
	for _, m := range cl.Members() {
		var live *player.Character
		if m.Online && online != nil {
			live = online(m.ObjectID)
		}
		if removed, ok := s.remove(cl, m.ObjectID, 0, live, now); ok {
			out.Members = append(out.Members, removed)
		}
	}
	cl.mu.Lock()
	clanID := cl.id
	s.write(clanID, "delete clan", func(ctx context.Context, st Store) error { return st.DeleteClan(ctx, clanID, castleID) })
	cl.mu.Unlock()
	return out, true
}

// forgetWars drops every war cl declared or had declared on it from the
// other clan's lists; the stored rows go with cl's (DeleteClan).
func (s *Service) forgetWars(cl *Clan) {
	cl.mu.RLock()
	related := make([]int32, 0, len(cl.wars)+len(cl.attackers))
	for id := range cl.wars {
		related = append(related, id)
	}
	for id := range cl.attackers {
		related = append(related, id)
	}
	cl.mu.RUnlock()
	for _, id := range related {
		other, ok := s.table.Get(id)
		if !ok {
			continue
		}
		unlock := lockPair(cl, other)
		delete(other.wars, cl.id)
		delete(other.attackers, cl.id)
		delete(cl.wars, id)
		delete(cl.attackers, id)
		unlock()
	}
}

// ApplyDispersed clears the clan state of m, removed at now when its clan
// was dissolved, on its live character; it runs on that character's queue.
// No join penalty follows, and any the member held is lifted unless it was
// in the academy; the clan's leader may not found another clan for
// CreateDays.
func (s *Service) ApplyDispersed(c *player.Character, m Member, leader bool, now time.Time) {
	c.SetTitle("")
	c.SetClanID(0)
	c.SetWantsPeace(false)
	if m.PledgeType != SubunitAcademy {
		c.SetClanJoinExpiryTime(0)
	}
	if leader {
		c.SetClanCreateExpiryTime(now.UnixMilli() + int64(s.cfg.CreateDays)*dayMillis)
	}
	c.SetPledgeClass(s.pledgeClass(c))
}

// Dissolver carries out a scheduled dissolution that came due: it
// destroys the clan (Destroy with due set) and tells its members.
type Dissolver interface {
	DissolveDue(cl *Clan)
}

// dissolutions holds the timer of each clan whose dissolution is pending.
// mu guards every field; it may be taken while a clan's mu is held, never
// the other way round.
type dissolutions struct {
	mu     sync.Mutex
	queue  *sim.Queue
	out    Dissolver
	timers map[int32]*sim.Timer
}

// StartDissolutions runs the pending dissolutions on queue from now on,
// handing each one that comes due to out. Every clan restored with a
// dissolution pending is destroyed once it expires, and a minute from now
// at the soonest. Before it runs, a requested dissolution is stored but
// never comes due.
func (s *Service) StartDissolutions(queue *sim.Queue, out Dissolver) {
	d := &s.dissolutions
	d.mu.Lock()
	d.queue, d.out = queue, out
	d.mu.Unlock()
	nowMs := queue.Now().UnixMilli()
	for _, cl := range s.table.allClans() {
		if expiry := cl.Info().DissolvingExpiry; expiry != 0 {
			d.schedule(cl, time.Duration(expiry-nowMs)*time.Millisecond)
		}
	}
}

// schedule arms cl's dissolution to come due after delay, a minute at the
// soonest, replacing the timer of an earlier request.
func (d *dissolutions) schedule(cl *Clan, delay time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.queue == nil {
		return
	}
	if t := d.timers[cl.id]; t != nil {
		t.Stop()
	}
	var timer *sim.Timer
	timer = d.queue.After(max(delay, minDissolveDelay), func() {
		d.mu.Lock()
		current := d.timers[cl.id] == timer
		if current {
			delete(d.timers, cl.id)
		}
		out := d.out
		d.mu.Unlock()
		if current && cl.Info().DissolvingExpiry != 0 {
			out.DissolveDue(cl)
		}
	})
	if d.timers == nil {
		d.timers = map[int32]*sim.Timer{}
	}
	d.timers[cl.id] = timer
}

// cancel stops the timer of clanID's dissolution, if one is armed.
func (d *dissolutions) cancel(clanID int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t := d.timers[clanID]; t != nil {
		t.Stop()
		delete(d.timers, clanID)
	}
}
