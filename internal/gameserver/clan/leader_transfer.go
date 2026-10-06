package clan

import (
	"cmp"
	"context"
	"slices"
)

// Handover is one clan leader change HandOverLeadership made: the clan,
// and its former and new leaders as they stand after it.
type Handover struct {
	Clan   *Clan
	Former Member
	Leader Member
}

// HandOverLeadership applies every clan's pending leader nomination, clan
// by clan in id order. A clan whose nominee has left it drops the
// nomination. Any other clan takes the nominee as its leader: the nominee
// gets the leader's power grade 0, the former leader the member grade 6,
// and the clan row, then both grades, are stored. The live state of the
// two characters (their clan rank, their equipment) is the caller's to
// refresh, on their own queues.
//
// Both grades change in the roster, an offline member's included, so the
// clan window shows them at once; the stored grades are what either member
// reads at its next login anyway.
func (s *Service) HandOverLeadership() []Handover {
	clans := s.table.allClans()
	slices.SortFunc(clans, func(a, b *Clan) int { return cmp.Compare(a.id, b.id) })
	var out []Handover
	for _, cl := range clans {
		if h, ok := s.handOver(cl); ok {
			out = append(out, h)
		}
	}
	return out
}

// handOver applies cl's pending nomination, reporting whether its leader
// changed.
func (s *Service) handOver(cl *Clan) (Handover, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.destroyed || cl.newLeaderID <= 0 {
		return Handover{}, false
	}
	leader, ok := cl.members[cl.newLeaderID]
	if !ok {
		cl.newLeaderID = 0
		s.updateClanLocked(cl)
		return Handover{}, false
	}
	formerID := cl.leaderID
	cl.leaderID = leader.ObjectID
	cl.newLeaderID = 0
	s.updateClanLocked(cl)

	leader.PowerGrade = LeaderPowerGrade
	former := Member{ObjectID: formerID}
	if m, ok := cl.members[formerID]; ok {
		m.PowerGrade = MemberPowerGrade
		former = *m
	}
	leaderID := leader.ObjectID
	s.write(leaderID, "set new leader power grade", func(ctx context.Context, st Store) error {
		return st.SetPowerGrade(ctx, leaderID, LeaderPowerGrade)
	})
	s.write(formerID, "set former leader power grade", func(ctx context.Context, st Store) error {
		return st.SetPowerGrade(ctx, formerID, MemberPowerGrade)
	})
	return Handover{Clan: cl, Former: former, Leader: *leader}, true
}
