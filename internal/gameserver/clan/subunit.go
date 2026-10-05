package clan

import (
	"context"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// SubPledge is one of a clan's sub-units: its academy, a royal guard or a
// knight order. ID is the pledge type its members carry.
type SubPledge struct {
	ID       int
	Name     string
	LeaderID int32
}

// SubunitRow is one clan_subpledges row.
type SubunitRow struct {
	ClanID int32
	SubPledge
}

// subunitOrder is the order a clan's sub-units are walked in wherever the
// client is sent one packet per sub-unit: the roster lists after the main
// clan's, at login, on request, on joining and on every status refresh.
var subunitOrder = []int{
	SubunitAcademy, SubunitKnight3, SubunitKnight4, SubunitRoyal1, SubunitRoyal2, SubunitKnight1, SubunitKnight2,
}

// The clan level each kind of sub-unit needs.
const (
	academyMinLevel = 5
	royalMinLevel   = 6
	knightMinLevel  = 7
)

// The reputation a royal guard and a knight order cost.
const (
	royalReputationCost  = 5000
	knightReputationCost = 10000
)

// maxCaptainNameLength bounds a named captain, in UTF-16 units.
const maxCaptainNameLength = 16

// The academy's age limits: a member above this level, or past its first
// class transfer, may not join it.
const (
	academyMaxLevel      = 40
	academyMaxClassLevel = 1
)

// The power grades a recruit takes in each kind of sub-unit.
const (
	academyPowerGrade = 9
	royalPowerGrade   = 7
	knightPowerGrade  = 8
)

// IsRoyal reports whether pledgeType is a royal guard.
func IsRoyal(pledgeType int) bool { return isRoyal(pledgeType) }

// IsKnight reports whether pledgeType is a knight order.
func IsKnight(pledgeType int) bool { return isKnight(pledgeType) }

// recruitPowerGrade is the power grade a recruit into pledgeType takes.
func recruitPowerGrade(pledgeType int) int {
	switch {
	case pledgeType == SubunitAcademy:
		return academyPowerGrade
	case isRoyal(pledgeType):
		return royalPowerGrade
	case isKnight(pledgeType):
		return knightPowerGrade
	}
	return MemberPowerGrade
}

// Subunit returns the sub-unit of pledge type id.
func (cl *Clan) Subunit(id int) (SubPledge, bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	sp, ok := cl.subunits[id]
	if !ok {
		return SubPledge{}, false
	}
	return *sp, true
}

// Subunits returns the clan's sub-units in the order the client is sent
// them.
func (cl *Clan) Subunits() []SubPledge {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	out := make([]SubPledge, 0, len(cl.subunits))
	for _, id := range subunitOrder {
		if sp, ok := cl.subunits[id]; ok {
			out = append(out, *sp)
		}
	}
	return out
}

// SubunitLeaderName is the name the roster of pledgeType shows as its
// leader: the clan leader's for the main clan, the captain's for a royal
// guard or knight order, "" for the academy, a vacant captaincy, a captain
// no longer on the roster or a sub-unit that does not exist.
func (cl *Clan) SubunitLeaderName(pledgeType int) string {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.subunitLeaderNameLocked(pledgeType)
}

func (cl *Clan) subunitLeaderNameLocked(pledgeType int) string {
	leaderID := cl.leaderID
	if pledgeType != SubunitMain {
		sp, ok := cl.subunits[pledgeType]
		if !ok || sp.ID == SubunitAcademy || sp.LeaderID == 0 {
			return ""
		}
		leaderID = sp.LeaderID
	}
	if m, ok := cl.members[leaderID]; ok {
		return m.Name
	}
	return ""
}

// subunitNamedLocked returns the sub-unit named name, ignoring case.
func (cl *Clan) subunitNamedLocked(name string) (*SubPledge, bool) {
	for _, id := range subunitOrder {
		if sp, ok := cl.subunits[id]; ok && strings.EqualFold(sp.Name, name) {
			return sp, true
		}
	}
	return nil, false
}

// leadsSubunitLocked returns the sub-unit objectID captains, 0 when none.
func (cl *Clan) leadsSubunitLocked(objectID int32) int {
	led := 0
	for _, id := range subunitOrder {
		if sp, ok := cl.subunits[id]; ok && sp.LeaderID != 0 && sp.LeaderID == objectID {
			led = id
		}
	}
	return led
}

// availableSubunitLocked returns the first free pledge type of the kind
// kind starts, 0 when every one is taken: one academy, two royal guards,
// four knight orders.
func (cl *Clan) availableSubunitLocked(kind int) int {
	next := map[int]int{SubunitRoyal1: SubunitRoyal2, SubunitKnight1: SubunitKnight2, SubunitKnight2: SubunitKnight3, SubunitKnight3: SubunitKnight4}
	for id := kind; id != 0; id = next[id] {
		if _, taken := cl.subunits[id]; !taken {
			return id
		}
	}
	return 0
}

// restoreSubunits files the stored sub-units under their clans; t.mu is
// held.
func (t *Table) restoreSubunits(rows []SubunitRow) {
	for _, r := range rows {
		if cl, ok := t.clans[r.ClanID]; ok {
			sp := r.SubPledge
			cl.subunits[sp.ID] = &sp
		}
	}
}

// subunitNameTaken reports whether any clan has a sub-unit named name,
// ignoring case.
func (t *Table) subunitNameTaken(name string) bool {
	t.mu.RLock()
	clans := make([]*Clan, 0, len(t.clans))
	for _, cl := range t.clans {
		clans = append(clans, cl)
	}
	t.mu.RUnlock()
	for _, cl := range clans {
		cl.mu.RLock()
		_, taken := cl.subunitNamedLocked(name)
		cl.mu.RUnlock()
		if taken {
			return true
		}
	}
	return false
}

// SubunitResult is the outcome of founding, renaming or staffing a
// sub-unit.
type SubunitResult int

// The sub-unit outcomes, in the order they are checked.
const (
	SubunitDone SubunitResult = iota
	SubunitNotLeader
	// SubunitLevelTooLow refuses a clan below the sub-unit's level.
	SubunitLevelTooLow
	SubunitNameInvalid
	SubunitNameLength
	// SubunitNameTaken refuses a name another sub-unit, of any clan,
	// carries.
	SubunitNameTaken
	// SubunitCaptainInvalid refuses a captain who is not a main-clan member,
	// or who already captains a sub-unit.
	SubunitCaptainInvalid
	// SubunitNoSlot refuses a sub-unit of a kind the clan has no room for.
	SubunitNoSlot
	// SubunitCaptainIsLeader refuses the clan leader as a captain.
	SubunitCaptainIsLeader
	SubunitReputationTooLow
	// SubunitUnknown answers a rename of a sub-unit that does not exist.
	SubunitUnknown
	// SubunitIgnored is not answered.
	SubunitIgnored
	// SubunitCaptainNameTooLong refuses a captain name over 16 characters.
	SubunitCaptainNameTooLong
	// SubunitCaptainSelf refuses the leader naming itself captain.
	SubunitCaptainSelf
	// SubunitNotMilitary refuses a captain for the academy or for a
	// sub-unit name the clan does not have.
	SubunitNotMilitary
)

// SubunitChange is a sub-unit just founded or given a captain.
type SubunitChange struct {
	Clan *Clan
	Unit SubPledge
	// Captain is the captain's roster row; zero for the academy.
	Captain Member
	// Reputation is the clan's reputation after the founding's cost; set
	// when Paid.
	Reputation ReputationChanged
	Paid       bool
}

// CreateSubunit founds a sub-unit of kind kind (SubunitAcademy,
// SubunitRoyal1 or SubunitKnight1) named name in c's clan, on its leader
// c's order. A royal guard or knight order is captained by the main-clan
// member named captainName and costs reputation.
func (s *Service) CreateSubunit(c *player.Character, kind int, name, captainName string) (SubunitChange, SubunitResult) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return SubunitChange{}, SubunitNotLeader
	}
	minLevel := academyMinLevel
	switch kind {
	case SubunitRoyal1:
		minLevel = royalMinLevel
	case SubunitKnight1:
		minLevel = knightMinLevel
	}
	switch {
	case cl.Level() < minLevel:
		return SubunitChange{}, SubunitLevelTooLow
	case !validName(name):
		return SubunitChange{}, SubunitNameInvalid
	case nameLength(name) < minNameLength || nameLength(name) > maxNameLength:
		return SubunitChange{}, SubunitNameLength
	}
	// One founding at a time across every clan, so two clans cannot both
	// take one free sub-unit name.
	s.subunitMu.Lock()
	defer s.subunitMu.Unlock()
	if s.table.subunitNameTaken(name) {
		return SubunitChange{}, SubunitNameTaken
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()
	var captain Member
	if kind != SubunitAcademy {
		m, ok := cl.memberByNameLocked(captainName)
		if !ok || m.PledgeType != SubunitMain {
			return SubunitChange{}, SubunitCaptainInvalid
		}
		captain = m
	}
	pledgeType := cl.availableSubunitLocked(kind)
	switch {
	case pledgeType == 0:
		return SubunitChange{}, SubunitNoSlot
	case cl.leaderID == captain.ObjectID:
		return SubunitChange{}, SubunitCaptainIsLeader
	case isRoyal(pledgeType) && cl.reputation < royalReputationCost,
		isKnight(pledgeType) && cl.reputation < knightReputationCost:
		return SubunitChange{}, SubunitReputationTooLow
	}
	sp := &SubPledge{ID: pledgeType, Name: name, LeaderID: captain.ObjectID}
	cl.subunits[pledgeType] = sp
	row := SubunitRow{ClanID: cl.id, SubPledge: *sp}
	s.write(cl.id, "store sub-unit", func(ctx context.Context, st Store) error { return st.InsertSubunit(ctx, row) })
	change := SubunitChange{Clan: cl, Unit: *sp, Captain: captain}
	cost := 0
	switch {
	case isRoyal(pledgeType):
		cost = royalReputationCost
	case isKnight(pledgeType):
		cost = knightReputationCost
	}
	if cost > 0 {
		change.Reputation, change.Paid = s.addReputationLocked(cl, -cost)
	}
	return change, SubunitDone
}

// RenameSubunit renames the sub-unit whose pledge type idArg spells to
// name, on its leader c's order. An id that is not a number is ignored.
func (s *Service) RenameSubunit(c *player.Character, idArg, name string) (SubunitChange, SubunitResult) {
	cl, ok := s.ClanOf(c)
	if !ok || !cl.IsLeader(c.ID) {
		return SubunitChange{}, SubunitNotLeader
	}
	id, err := commons.ParseInt(idArg, 32)
	if err != nil {
		return SubunitChange{}, SubunitIgnored
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	sp, ok := cl.subunits[int(id)]
	switch {
	case !ok:
		return SubunitChange{}, SubunitUnknown
	case !validName(name):
		return SubunitChange{}, SubunitNameInvalid
	case nameLength(name) < minNameLength || nameLength(name) > maxNameLength:
		return SubunitChange{}, SubunitNameLength
	}
	sp.Name = name
	s.updateSubunitLocked(cl, *sp)
	return SubunitChange{Clan: cl, Unit: *sp}, SubunitDone
}

// AssignSubunitCaptain names the main-clan member called captainName
// captain of c's clan's royal guard or knight order named unitName, on its
// leader c's order.
func (s *Service) AssignSubunitCaptain(c *player.Character, unitName, captainName string) (SubunitChange, SubunitResult) {
	cl, ok := s.ClanOf(c)
	switch {
	case !ok || !cl.IsLeader(c.ID):
		return SubunitChange{}, SubunitNotLeader
	case nameLength(captainName) > maxCaptainNameLength:
		return SubunitChange{}, SubunitCaptainNameTooLong
	case c.Name == captainName:
		return SubunitChange{}, SubunitCaptainSelf
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	sp, ok := cl.subunitNamedLocked(unitName)
	if !ok || sp.ID == SubunitAcademy {
		return SubunitChange{}, SubunitNotMilitary
	}
	change := SubunitChange{Clan: cl, Unit: *sp}
	m, ok := cl.memberByNameLocked(captainName)
	if !ok || m.PledgeType != SubunitMain || cl.leadsSubunitLocked(m.ObjectID) != 0 {
		return change, SubunitCaptainInvalid
	}
	sp.LeaderID = m.ObjectID
	s.updateSubunitLocked(cl, *sp)
	change.Unit, change.Captain = *sp, m
	return change, SubunitDone
}

// updateSubunitLocked queues sp's clan_subpledges row; cl.mu is held.
func (s *Service) updateSubunitLocked(cl *Clan, sp SubPledge) {
	row := SubunitRow{ClanID: cl.id, SubPledge: sp}
	s.write(cl.id, "update sub-unit", func(ctx context.Context, st Store) error { return st.UpdateSubunit(ctx, row) })
}

// RefreshPledgeClass recomputes c's clan rank from its clan membership.
func (s *Service) RefreshPledgeClass(c *player.Character) {
	c.SetPledgeClass(s.pledgeClass(c))
}

// ReorganizeResult is the outcome of moving a member between sub-units.
type ReorganizeResult int

// The reorganization outcomes. ReorganizeShowMember answers with the named
// member's card, which puts back the affiliation the client already
// changed on its side.
const (
	Reorganized ReorganizeResult = iota
	ReorganizeIgnored
	ReorganizeNotAuthorized
	ReorganizeShowMember
)

// Reorganize moves the member called name into sub-unit pledgeType and
// the member called swapName into name's old sub-unit, on c's order. With
// selected false it only shows name's card. A sub-unit captain stays in
// the main clan. A move into a sub-unit the clan does not have is refused
// with name's card: a crafted request could otherwise hide a member in a
// sub-unit no roster lists.
func (s *Service) Reorganize(c *player.Character, selected bool, name string, pledgeType int, swapName string) (*Clan, Member, ReorganizeResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return nil, Member{}, ReorganizeIgnored
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.memberPrivilegesLocked(c.ID)&int32(PrivManageRanks) == 0 {
		return nil, Member{}, ReorganizeNotAuthorized
	}
	m1, ok1 := cl.memberByNameLocked(name)
	if !selected {
		if ok1 {
			return cl, m1, ReorganizeShowMember
		}
		return nil, Member{}, ReorganizeIgnored
	}
	m2, ok2 := cl.memberByNameLocked(swapName)
	switch {
	case !ok1 || m1.ObjectID == cl.leaderID || !ok2 || m2.ObjectID == cl.leaderID:
		return nil, Member{}, ReorganizeIgnored
	case cl.leadsSubunitLocked(m1.ObjectID) != 0:
		return cl, m1, ReorganizeShowMember
	case m1.PledgeType == pledgeType:
		return nil, Member{}, ReorganizeIgnored
	}
	if _, exists := cl.subunits[pledgeType]; pledgeType != SubunitMain && !exists {
		return cl, m1, ReorganizeShowMember
	}
	old := m1.PledgeType
	cl.members[m1.ObjectID].PledgeType = pledgeType
	cl.members[m2.ObjectID].PledgeType = old
	for _, id := range []int32{m1.ObjectID, m2.ObjectID} {
		objectID, unit := id, cl.members[id].PledgeType
		s.write(objectID, "set member sub-unit", func(ctx context.Context, st Store) error {
			return st.SetPledgeType(ctx, objectID, unit)
		})
	}
	return cl, *cl.members[m1.ObjectID], Reorganized
}

// MentorResult is the outcome of linking or unlinking an academy member
// and its sponsor.
type MentorResult int

// The mentor outcomes. MentorIgnored is not answered.
const (
	MentorLinked MentorResult = iota
	MentorUnlinked
	MentorIgnored
	MentorNotAuthorized
	// MentorAlreadyLinked refuses a link while either side has one.
	MentorAlreadyLinked
)

// MentorChange is the sponsor and apprentice of a link change, as their
// rows stand afterwards.
type MentorChange struct {
	Clan       *Clan
	Sponsor    Member
	Apprentice Member
}

// SetMentor links the members called name and otherName as sponsor and
// apprentice, or unlinks them when link is false, on c's order. The
// academy member of the two is the apprentice; when name is not in the
// academy otherName is. Unlinking clears both members' links, as their
// stored rows are.
func (s *Service) SetMentor(c *player.Character, link bool, name, otherName string) (MentorChange, MentorResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return MentorChange{}, MentorIgnored
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.memberPrivilegesLocked(c.ID)&int32(PrivMasterRights) == 0 {
		return MentorChange{}, MentorNotAuthorized
	}
	current, ok1 := cl.memberByNameLocked(name)
	other, ok2 := cl.memberByNameLocked(otherName)
	if !ok1 || !ok2 {
		return MentorChange{}, MentorIgnored
	}
	apprentice, sponsor := cl.members[other.ObjectID], cl.members[current.ObjectID]
	if current.PledgeType == SubunitAcademy {
		apprentice, sponsor = cl.members[current.ObjectID], cl.members[other.ObjectID]
	}
	result := MentorUnlinked
	if link {
		if apprentice.Sponsor != 0 || sponsor.Apprentice != 0 || apprentice.Apprentice != 0 || sponsor.Sponsor != 0 {
			return MentorChange{}, MentorAlreadyLinked
		}
		apprentice.Apprentice, apprentice.Sponsor = 0, sponsor.ObjectID
		sponsor.Apprentice, sponsor.Sponsor = apprentice.ObjectID, 0
		result = MentorLinked
	} else {
		apprentice.Apprentice, apprentice.Sponsor = 0, 0
		sponsor.Apprentice, sponsor.Sponsor = 0, 0
	}
	s.saveMentorLocked(*apprentice)
	s.saveMentorLocked(*sponsor)
	return MentorChange{Clan: cl, Sponsor: *sponsor, Apprentice: *apprentice}, result
}

// saveMentorLocked queues m's apprentice and sponsor columns; the clan's
// mu is held.
func (s *Service) saveMentorLocked(m Member) {
	objectID, apprentice, sponsor := m.ObjectID, m.Apprentice, m.Sponsor
	s.write(objectID, "save apprentice and sponsor", func(ctx context.Context, st Store) error {
		return st.SetMentor(ctx, objectID, apprentice, sponsor)
	})
}

// unlinkLeaverLocked undoes what m, leaving cl, held: a captaincy falls
// vacant, and its apprentice and sponsor lose their link to it. cl.mu is
// held.
func (s *Service) unlinkLeaverLocked(cl *Clan, m *Member) {
	if led := cl.leadsSubunitLocked(m.ObjectID); led != 0 {
		sp := cl.subunits[led]
		sp.LeaderID = 0
		s.updateSubunitLocked(cl, *sp)
	}
	for _, id := range []int32{m.Apprentice, m.Sponsor} {
		if id == 0 {
			continue
		}
		if linked, ok := cl.members[id]; ok {
			linked.Apprentice, linked.Sponsor = 0, 0
			s.saveMentorLocked(*linked)
		}
	}
}
