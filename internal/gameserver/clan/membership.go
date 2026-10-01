package clan

import (
	"context"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// minCreateLevel is the character level a clan founder needs.
const minCreateLevel = 10

// Clan name length bounds, in UTF-16 units.
const (
	minNameLength = 2
	maxNameLength = 16
)

// CreateResult is the outcome of founding a clan.
type CreateResult int

// The founding outcomes, in the order they are checked.
const (
	Created CreateResult = iota
	CreateLevelTooLow
	CreateAlreadyInClan
	CreateMustWait
	CreateNameInvalid
	CreateNameLength
	// CreateNameTaken refuses a name another clan carries, ignoring case.
	CreateNameTaken
	// CreateFailed is an id allocation failure.
	CreateFailed
)

// Create founds a clan named name with c as its leader.
func (s *Service) Create(c *player.Character, name string, now time.Time) (*Clan, CreateResult) {
	switch {
	case c.Level() < minCreateLevel:
		return nil, CreateLevelTooLow
	case c.ClanID() != 0:
		return nil, CreateAlreadyInClan
	case now.UnixMilli() < c.ClanCreateExpiryTime():
		return nil, CreateMustWait
	case !validName(name):
		return nil, CreateNameInvalid
	case nameLength(name) < minNameLength || nameLength(name) > maxNameLength:
		return nil, CreateNameLength
	}
	if _, taken := s.table.ByName(name); taken {
		return nil, CreateNameTaken
	}
	if s.ids == nil {
		return nil, CreateFailed
	}
	id, err := s.ids.NextID()
	if err != nil {
		s.log.Error().Err(err).Msg("clan: allocate clan id")
		return nil, CreateFailed
	}
	cl := &Clan{
		id: id, name: name, leaderID: c.ID,
		members:    map[int32]*Member{},
		privileges: map[int]int32{},
	}
	leader := LiveMember(c)
	leader.Title = ""
	leader.PowerGrade = LeaderPowerGrade
	leader.PledgeType = SubunitMain
	leader.Online = true
	cl.members[c.ID] = &leader
	if !s.table.insert(cl) {
		return nil, CreateNameTaken
	}
	// Only the founder, whose queue this runs on, can act on the clan yet;
	// its rows are still queued under cl.mu like every later change, so
	// they reach the store first.
	cl.mu.Lock()
	clanRow := Row{ID: id, Name: name, LeaderID: c.ID}
	s.write(id, "store new clan", func(ctx context.Context, st Store) error { return st.InsertClan(ctx, clanRow) })
	s.saveMembershipLocked(MembershipRow{
		ObjectID: c.ID, ClanID: id, PowerGrade: leader.PowerGrade, PledgeType: leader.PledgeType,
		JoinExpiry: c.ClanJoinExpiryTime(),
	})
	cl.mu.Unlock()

	c.SetClanID(id)
	c.SetTitle("")
	c.SetPledgeClass(s.pledgeClass(c))
	return cl, Created
}

// LiveMember is c's roster row from its live values, for a member joining,
// founding, or being marked online or offline.
func LiveMember(c *player.Character) Member {
	return Member{
		ObjectID: c.ID, Name: c.Name, Title: c.Title(),
		Level: c.Level(), ClassID: c.ClassID(), Sex: int(c.Sex), Race: int(c.Race),
	}
}

// saveMembershipLocked queues row; the clan's mu is held, so the row lands
// before any removal of the same member that follows it.
func (s *Service) saveMembershipLocked(row MembershipRow) {
	s.write(row.ObjectID, "save clan membership", func(ctx context.Context, st Store) error { return st.SaveMembership(ctx, row) })
}

// JoinRefusal is why a clan invitation, or its acceptance, is refused.
type JoinRefusal int

// The invitation refusals, in the order they are checked. JoinNotAuthorized
// and the rest are answered to the inviter.
const (
	JoinAllowed JoinRefusal = iota
	JoinNotAuthorized
	JoinInviteSelf
	JoinTargetInClan
	JoinClanPenalty
	JoinTargetPenalty
	JoinClanFull
	JoinSubunitFull
)

// CheckJoin reports whether inviterID may invite target into cl's sub-unit
// pledgeType now.
func (s *Service) CheckJoin(cl *Clan, inviterID int32, target *player.Character, pledgeType int, now time.Time) JoinRefusal {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.checkJoinLocked(inviterID, target, pledgeType, now)
}

// checkJoinLocked runs the invitation rules. The target's block list is
// not modeled yet (#3151), so a blocking target is never refused. Only the
// main clan recruits: sub-units are not loaded yet (#149), so an
// invitation into one finds no room.
func (cl *Clan) checkJoinLocked(inviterID int32, target *player.Character, pledgeType int, now time.Time) JoinRefusal {
	nowMs := now.UnixMilli()
	switch {
	case cl.memberPrivilegesLocked(inviterID)&int32(PrivInvite) == 0:
		return JoinNotAuthorized
	case inviterID == target.ID:
		return JoinInviteSelf
	case target.ClanID() != 0:
		return JoinTargetInClan
	case cl.charPenaltyExpiry > nowMs:
		return JoinClanPenalty
	case target.ClanJoinExpiryTime() > nowMs:
		return JoinTargetPenalty
	case pledgeType != SubunitMain:
		return JoinSubunitFull
	case cl.subunitCountLocked(pledgeType) >= MaxMembers(cl.level, pledgeType):
		return JoinClanFull
	}
	return JoinAllowed
}

// Join puts c into cl's sub-unit pledgeType on inviterID's invitation,
// re-checking the invitation rules first. A main-clan recruit takes rank 6.
func (s *Service) Join(cl *Clan, inviterID int32, c *player.Character, pledgeType int, now time.Time) JoinRefusal {
	m := LiveMember(c)
	m.PledgeType = pledgeType
	m.PowerGrade = MemberPowerGrade
	m.Online = true
	cl.mu.Lock()
	if refusal := cl.checkJoinLocked(inviterID, c, pledgeType, now); refusal != JoinAllowed {
		cl.mu.Unlock()
		return refusal
	}
	m.Title = ""
	cl.members[c.ID] = &m
	s.saveMembershipLocked(MembershipRow{ObjectID: c.ID, ClanID: cl.id, PowerGrade: m.PowerGrade, PledgeType: m.PledgeType})
	cl.mu.Unlock()

	c.SetClanID(cl.id)
	c.SetTitle("")
	c.SetPledgeClass(s.pledgeClass(c))
	c.SetClanJoinExpiryTime(0)
	return JoinAllowed
}

// LeaveRefusal is why a member may not withdraw.
type LeaveRefusal int

// The withdrawal refusals, in the order they are checked.
const (
	Left LeaveRefusal = iota
	LeaveNotMember
	LeaveIsLeader
	LeaveInCombat
)

// Withdraw takes c out of its clan, which it may not rejoin, nor join
// another, for JoinDays.
func (s *Service) Withdraw(c *player.Character, now time.Time) (*Clan, Member, LeaveRefusal) {
	cl, ok := s.ClanOf(c)
	switch {
	case !ok:
		return nil, Member{}, LeaveNotMember
	case cl.IsLeader(c.ID):
		return nil, Member{}, LeaveIsLeader
	case c.InCombat():
		return nil, Member{}, LeaveInCombat
	}
	joinExpiry := s.joinExpiry(now)
	m, removed := s.remove(cl, c.ID, joinExpiry, c, now)
	if !removed {
		return nil, Member{}, LeaveNotMember
	}
	s.ApplyLeft(c, m, now)
	return cl, m, Left
}

// OustRefusal is why an expulsion is refused.
type OustRefusal int

// The expulsion refusals, in the order they are checked.
const (
	Ousted OustRefusal = iota
	OustNotMember
	// OustUnknownTarget is a name no member carries; it is not answered.
	OustUnknownTarget
	OustNotAuthorized
	OustSelf
	OustInCombat
)

// Oust expels the member named targetName from c's clan. online resolves a
// member's live character; nil when it is not in the world. The expelled
// member may not join a clan, nor the clan recruit, for JoinDays. The
// leader cannot be expelled: an expulsion naming it is refused as
// unauthorized, as a clan without its leader would be left unusable.
func (s *Service) Oust(c *player.Character, targetName string, online func(int32) *player.Character, now time.Time) (*Clan, Member, OustRefusal) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return nil, Member{}, OustNotMember
	}
	target, ok := cl.MemberByName(targetName)
	switch {
	case !ok:
		return nil, Member{}, OustUnknownTarget
	case !cl.HasPrivilege(c.ID, PrivDismiss):
		return nil, Member{}, OustNotAuthorized
	case strings.EqualFold(c.Name, targetName):
		return nil, Member{}, OustSelf
	case cl.IsLeader(target.ObjectID):
		return nil, Member{}, OustNotAuthorized
	}
	var live *player.Character
	if target.Online {
		live = online(target.ObjectID)
	}
	if live != nil && live.InCombat() {
		return nil, Member{}, OustInCombat
	}
	joinExpiry := s.joinExpiry(now)
	m, removed := s.remove(cl, target.ObjectID, joinExpiry, live, now)
	if !removed {
		return nil, Member{}, OustUnknownTarget
	}
	cl.mu.Lock()
	cl.charPenaltyExpiry = joinExpiry
	s.updateClanLocked(cl)
	cl.mu.Unlock()
	return cl, m, Ousted
}

// remove drops objectID from cl's roster and queues its row under cl.mu:
// live is its character when online, nil otherwise.
func (s *Service) remove(cl *Clan, objectID int32, joinExpiry int64, live *player.Character, now time.Time) (Member, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	m, ok := cl.members[objectID]
	wasLeader := objectID == cl.leaderID
	if !ok {
		return Member{}, false
	}
	delete(cl.members, objectID)
	row := RemovalRow{ObjectID: objectID, JoinExpiry: joinExpiry, Online: live != nil}
	switch {
	case wasLeader:
		row.CreateExpiry = now.UnixMilli() + int64(s.cfg.CreateDays)*dayMillis
	case live != nil:
		row.CreateExpiry = live.ClanCreateExpiryTime()
		if m.PledgeType == SubunitAcademy {
			row.JoinExpiry = live.ClanJoinExpiryTime()
		}
	}
	s.write(objectID, "remove clan member", func(ctx context.Context, st Store) error { return st.RemoveMembership(ctx, row) })
	return *m, true
}

// ApplyLeft clears the clan state of m, removed at now, on its live
// character; it runs on that character's queue. An academy member leaves
// without a join penalty.
func (s *Service) ApplyLeft(c *player.Character, m Member, now time.Time) {
	c.SetTitle("")
	c.SetClanID(0)
	if m.PledgeType != SubunitAcademy {
		c.SetClanJoinExpiryTime(s.joinExpiry(now))
	}
	c.SetPledgeClass(s.pledgeClass(c))
}

// joinExpiry is when a join penalty starting at now ends.
func (s *Service) joinExpiry(now time.Time) int64 {
	return now.UnixMilli() + int64(s.cfg.JoinDays)*dayMillis
}

// RemoveDeleted takes a character deleted for good off its clan's roster.
func (s *Service) RemoveDeleted(objectID int32, now time.Time) {
	if cl, ok := s.table.MemberClan(objectID); ok {
		s.remove(cl, objectID, 0, nil, now)
	}
}
