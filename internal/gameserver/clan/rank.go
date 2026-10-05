package clan

import (
	"context"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// Power grade bounds a member can be given; 0 is the leader's.
const (
	minMemberGrade = 1
	maxMemberGrade = 9
)

// GradeResult is the outcome of setting a member's power grade.
type GradeResult int

// The power grade outcomes. GradeIgnored is not answered.
const (
	GradeSet GradeResult = iota
	GradeIgnored
	GradeNotAuthorized
)

// SetMemberGrade gives the member named name power grade grade on c's
// order. A name no member carries, an academy member and a grade outside
// 1-9 are ignored. Only a member holding the rank-management privilege may
// change grades: without that gate any member could raise its own grade
// and take every privilege the clan gave it.
func (s *Service) SetMemberGrade(c *player.Character, name string, grade int) (*Clan, Member, GradeResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return nil, Member{}, GradeIgnored
	}
	cl.mu.Lock()
	target, ok := cl.memberByNameLocked(name)
	switch {
	case !ok, target.PledgeType == SubunitAcademy, grade < minMemberGrade, grade > maxMemberGrade:
		cl.mu.Unlock()
		return nil, Member{}, GradeIgnored
	case cl.memberPrivilegesLocked(c.ID)&int32(PrivManageRanks) == 0:
		cl.mu.Unlock()
		return nil, Member{}, GradeNotAuthorized
	}
	m := cl.members[target.ObjectID]
	m.PowerGrade = grade
	updated := *m
	s.write(updated.ObjectID, "set member power grade", func(ctx context.Context, st Store) error {
		return st.SetPowerGrade(ctx, updated.ObjectID, grade)
	})
	cl.mu.Unlock()
	return cl, updated, GradeSet
}

// SetRankPrivileges gives power grade rank the privileges privs on c's
// order. Only the leader may, only for ranks 1-9; the academy rank keeps
// the warehouse-search, entry and function privileges alone. It reports
// whether the privileges changed.
func (s *Service) SetRankPrivileges(c *player.Character, rank int, privs int32) (*Clan, bool) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) || rank < minPrivilegeRank || rank > academyRank {
		return nil, false
	}
	if rank == academyRank {
		privs &= int32(academyPrivilegeMask)
	}
	cl.mu.Lock()
	cl.privileges[rank] = privs
	s.write(cl.id, "store rank privileges", func(ctx context.Context, st Store) error {
		return st.SetPrivileges(ctx, cl.id, rank, privs)
	})
	cl.mu.Unlock()
	return cl, true
}

// NominationResult is the outcome of naming the next clan leader.
type NominationResult int

// The nomination outcomes. NominateSelf is not answered.
const (
	Nominated NominationResult = iota
	NominationPending
	NominateNotLeader
	NominateSelf
	NominateUnknown
	NominateOffline
	NominateOutsideMainClan
)

// NominateLeader names the member called name as the clan's next leader,
// on its leader c's request. The handover itself is a scheduled task
// (#3149); until it runs a second nomination is refused as pending.
// connected reports whether a member in the world still has its client:
// one lingering after its connection dropped is refused as offline
// (ClanMember.isOnline). It is called with the clan lock released.
func (s *Service) NominateLeader(c *player.Character, name string, connected func(objectID int32) bool) NominationResult {
	cl, ok := s.ClanOf(c)
	switch {
	case !ok || !cl.IsLeader(c.ID):
		return NominateNotLeader
	case strings.EqualFold(c.Name, name):
		return NominateSelf
	}
	cl.mu.Lock()
	m, ok := cl.memberByNameLocked(name)
	switch {
	case !ok:
		cl.mu.Unlock()
		return NominateUnknown
	case !m.Online:
		cl.mu.Unlock()
		return NominateOffline
	}
	nominee := m.ObjectID
	cl.mu.Unlock()
	if !connected(nominee) {
		return NominateOffline
	}
	// The member is read again: it may have left the clan or the world
	// while the lock was released.
	cl.mu.Lock()
	m, ok = cl.memberByNameLocked(name)
	switch {
	case !ok || m.ObjectID != nominee:
		cl.mu.Unlock()
		return NominateUnknown
	case !m.Online:
		cl.mu.Unlock()
		return NominateOffline
	case m.PledgeType != SubunitMain:
		cl.mu.Unlock()
		return NominateOutsideMainClan
	case cl.newLeaderID != 0:
		cl.mu.Unlock()
		return NominationPending
	}
	cl.newLeaderID = m.ObjectID
	s.updateClanLocked(cl)
	cl.mu.Unlock()
	return Nominated
}

// CancelResult is the outcome of withdrawing a leader nomination.
type CancelResult int

// The cancellation outcomes.
const (
	NominationCancelled CancelResult = iota
	NoNomination
	CancelNotLeader
)

// CancelNomination withdraws the clan's pending leader nomination on its
// leader c's request.
func (s *Service) CancelNomination(c *player.Character) CancelResult {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return CancelNotLeader
	}
	cl.mu.Lock()
	if cl.newLeaderID == 0 {
		cl.mu.Unlock()
		return NoNomination
	}
	cl.newLeaderID = 0
	s.updateClanLocked(cl)
	cl.mu.Unlock()
	return NominationCancelled
}
