package clan

import (
	"context"
	"slices"
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
	// DissolveSiegeRegistered refuses a clan registered on a castle siege
	// not under way yet; DissolveInSiege one whose siege is under way.
	DissolveSiegeRegistered
	DissolveInSiege
	DissolveInProgress
)

// RequestDissolve has c, its clan's leader, ask for the clan to be
// dissolved as of now. A clan in an alliance, at war, or owning a castle or
// clan hall may not be dissolved, nor one registered on a castle siege
// (sieges; the first siege it is registered on decides the refusal), nor
// one whose dissolution is already pending.
func (s *Service) RequestDissolve(c *player.Character, now time.Time, sieges Sieges) (*Clan, DissolveResult) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return nil, DissolveNotLeader
	}
	// Read before the clan's lock is taken: the sieges read the clans.
	registered, inSiege := siegeRegistration(sieges, cl.id)
	nowMs := now.UnixMilli()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	switch {
	case cl.destroyed:
		return nil, DissolveNotLeader
	case cl.allyID != 0:
		return cl, DissolveInAlliance
	case cl.wars.Len() > 0:
		return cl, DissolveAtWar
	case cl.castleID > 0 || cl.hallID > 0:
		return cl, DissolveOwnsResidence
	case registered && inSiege:
		return cl, DissolveInSiege
	case registered:
		return cl, DissolveSiegeRegistered
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
	// The timer of the recovered request stays armed: should the leader
	// ask again before it runs out, the clan goes at that earlier deadline.
	cl.dissolvingExpiry = 0
	s.updateClanLocked(cl)
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
// dissolution was called off since. Every clan at war with it, either
// way, forgets the war; every member leaves it as a member who withdrew
// does, without a join penalty, its leader with the creation penalty a
// leader leaving takes; then it leaves the registry and its rows go: the clan,
// its privileges, skills, sub-units, wars and siege registrations, and the
// tax of a castle it held is reset. online resolves a member's live
// character, nil when it is not in the world. The members' live state is
// the caller's to clear (ApplyDispersed), as are the clan warehouse and
// the clan's place on the sieges.
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
	s.dissolutions.cancel(cl.id)
	s.forgetWars(cl)

	// The members leave while cl is still registered: a member entering
	// the world meanwhile either finds itself still listed and enters
	// online, or finds itself gone with its removal kept for it, never a
	// clan missing with its removal still to come.
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
	s.table.remove(cl.id)
	cl.mu.Lock()
	clanID := cl.id
	s.write(clanID, "delete clan", func(ctx context.Context, st Store) error { return st.DeleteClan(ctx, clanID, castleID) })
	cl.mu.Unlock()
	return out, true
}

// forgetWars drops every war cl declared or had declared on it from the
// other clan's lists; the stored rows go with cl's (DeleteClan).
//
// Deliberate simplification: both directions are cleaned. The reference
// cleans only the clans that declared war on cl, so a clan cl had declared
// war on would keep the dissolved clan as an attacker until restart; that
// stale entry is not reproduced.
func (s *Service) forgetWars(cl *Clan) {
	cl.mu.RLock()
	related := make([]int32, 0, cl.wars.Len()+cl.attackers.Len())
	related = append(related, cl.wars.Order()...)
	related = append(related, cl.attackers.Order()...)
	cl.mu.RUnlock()
	for _, id := range related {
		other, ok := s.table.Get(id)
		if !ok {
			continue
		}
		unlock := lockPair(cl, other)
		other.wars.Remove(cl.id)
		other.attackers.Remove(cl.id)
		cl.wars.Remove(id)
		cl.attackers.Remove(id)
		unlock()
	}
}

// ApplyDispersed clears the clan state of m, removed at now when its clan
// was dissolved, on its live character; it runs on that character's queue.
// Its title and siege state clear; a personal surrender stays. No join
// penalty follows, and any the member held is lifted unless it was in the academy;
// the clan's leader may not found another clan for CreateDays.
func (s *Service) ApplyDispersed(c *player.Character, m Member, leader bool, now time.Time) {
	c.SetTitle("")
	c.SetClanID(0)
	c.SetSiegeState(player.SiegeStateNone)
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

// dissolutions holds the timers of each clan whose dissolution was
// requested or restored. mu guards every field; it may be taken while a
// clan's mu is held, never the other way round.
type dissolutions struct {
	mu     sync.Mutex
	queue  *sim.Queue
	out    Dissolver
	timers map[int32][]*sim.Timer
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

// schedule arms a timer for cl's dissolution after delay, a minute at the
// soonest, beside any earlier request's. Each timer, as it runs out, hands
// cl over if any dissolution is pending then, so a recovered request's
// timer still destroys a clan whose leader asked again since.
func (d *dissolutions) schedule(cl *Clan, delay time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.queue == nil {
		return
	}
	var timer *sim.Timer
	timer = d.queue.After(max(delay, minDissolveDelay), func() {
		d.mu.Lock()
		armed := slices.Index(d.timers[cl.id], timer)
		if armed >= 0 {
			d.timers[cl.id] = slices.Delete(d.timers[cl.id], armed, armed+1)
			if len(d.timers[cl.id]) == 0 {
				delete(d.timers, cl.id)
			}
		}
		out := d.out
		d.mu.Unlock()
		if armed >= 0 && cl.Info().DissolvingExpiry != 0 {
			out.DissolveDue(cl)
		}
	})
	if d.timers == nil {
		d.timers = map[int32][]*sim.Timer{}
	}
	d.timers[cl.id] = append(d.timers[cl.id], timer)
}

// cancel stops every timer of clanID's dissolution.
func (d *dissolutions) cancel(clanID int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.timers[clanID] {
		t.Stop()
	}
	delete(d.timers, clanID)
}
