// Package clan owns the clan registry: every clan's header, roster and rank
// privileges, the rules for founding, joining, leaving, expelling, ranking
// and levelling, and the rows those changes write. It decides and mutates;
// the network layer turns the returned outcomes into packets.
package clan

import (
	"sort"
	"sync"
)

// Sub-unit (pledge type) ids. The main clan is 0.
const (
	SubunitMain    = 0
	SubunitAcademy = -1
	SubunitRoyal1  = 100
	SubunitRoyal2  = 200
	SubunitKnight1 = 1001
	SubunitKnight2 = 1002
	SubunitKnight3 = 2001
	SubunitKnight4 = 2002
)

// LeaderPowerGrade and MemberPowerGrade are the ranks a clan's leader and a
// main-clan recruit hold.
const (
	LeaderPowerGrade = 0
	MemberPowerGrade = 6
)

// Member is one roster row. For an online member the network layer reads
// the level, class, sex, race and title off the live character instead;
// these fields are the values last known while it was offline.
type Member struct {
	ObjectID   int32
	Name       string
	Title      string
	Level      int
	ClassID    int
	Sex        int
	Race       int
	PledgeType int
	PowerGrade int
	Apprentice int32
	Sponsor    int32
	// LvlJoinedAcademy is the level the member joined the academy at; 0
	// for a member that never did, which is what marks an academy member.
	LvlJoinedAcademy int
	Online           bool
}

// Clan is one clan. mu guards every field: members join and leave on their
// own queues while other members' packets read the roster.
type Clan struct {
	mu sync.RWMutex

	id                int32
	name              string
	leaderID          int32
	newLeaderID       int32
	level             int
	castleID          int32
	hallID            int32
	crestID           int32
	crestLargeID      int32
	allyID            int32
	allyName          string
	allyCrestID       int32
	reputation        int
	rank              int
	allyPenaltyExpiry int64
	allyPenaltyType   int
	charPenaltyExpiry int64
	dissolvingExpiry  int64
	board             board

	members    map[int32]*Member
	privileges map[int]int32
	// skills maps each skill the clan learnt to its level.
	skills map[int]int
	// subunits are the academy, royal guards and knight orders the clan
	// founded, by pledge type.
	subunits map[int]*SubPledge
	// wars are the clans this clan declared war on; attackers the clans
	// that declared war on it; warPenalties when this clan may declare
	// war again on a clan it stopped a war with, in epoch milliseconds.
	wars         map[int32]struct{}
	attackers    map[int32]struct{}
	warPenalties map[int32]int64
}

// newClan returns a clan with empty rosters and registries.
func newClan(id int32, name string, leaderID int32) *Clan {
	return &Clan{
		id: id, name: name, leaderID: leaderID,
		members:      map[int32]*Member{},
		privileges:   map[int]int32{},
		skills:       map[int]int{},
		subunits:     map[int]*SubPledge{},
		wars:         map[int32]struct{}{},
		attackers:    map[int32]struct{}{},
		warPenalties: map[int32]int64{},
	}
}

// Info is a clan's header as the pledge window shows it.
type Info struct {
	ID         int32
	Name       string
	LeaderID   int32
	LeaderName string
	Level      int
	CastleID   int32
	HallID     int32
	CrestID    int32
	CrestLarge int32
	Rank       int
	Reputation int
	// DissolvingExpiry is the epoch millisecond a requested dissolution
	// completes, 0 when none is pending.
	DissolvingExpiry int64
	AllyID           int32
	AllyName         string
	AllyCrestID      int32
	// AllyPenaltyExpiry is the epoch millisecond the clan's alliance
	// penalty, of kind AllyPenaltyType, ends; 0 when it has none.
	AllyPenaltyExpiry int64
	AllyPenaltyType   int
	AtWar             bool
}

// ID is the clan's id.
func (cl *Clan) ID() int32 { return cl.id }

// Name is the clan's name.
func (cl *Clan) Name() string {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.name
}

// LeaderID is the object id of the clan's leader.
func (cl *Clan) LeaderID() int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.leaderID
}

// IsLeader reports whether objectID leads the clan.
func (cl *Clan) IsLeader(objectID int32) bool { return cl.LeaderID() == objectID }

// Level is the clan's level.
func (cl *Clan) Level() int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.level
}

// Info returns the clan's header.
func (cl *Clan) Info() Info {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.infoLocked()
}

func (cl *Clan) infoLocked() Info {
	info := Info{
		ID: cl.id, Name: cl.name, LeaderID: cl.leaderID,
		Level: cl.level, CastleID: cl.castleID, HallID: cl.hallID,
		CrestID: cl.crestID, CrestLarge: cl.crestLargeID,
		Rank: cl.rank, Reputation: cl.reputation,
		DissolvingExpiry: cl.dissolvingExpiry,
		AllyID:           cl.allyID, AllyName: cl.allyName, AllyCrestID: cl.allyCrestID,
		AllyPenaltyExpiry: cl.allyPenaltyExpiry, AllyPenaltyType: cl.allyPenaltyType,
		AtWar: len(cl.wars) > 0,
	}
	if leader, ok := cl.members[cl.leaderID]; ok {
		info.LeaderName = leader.Name
	}
	return info
}

// Member returns the roster row of objectID.
func (cl *Clan) Member(objectID int32) (Member, bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	m, ok := cl.members[objectID]
	if !ok {
		return Member{}, false
	}
	return *m, true
}

// IsMember reports whether objectID is on the roster.
func (cl *Clan) IsMember(objectID int32) bool {
	_, ok := cl.Member(objectID)
	return ok
}

// MemberByName returns the roster row named exactly name; the match is
// case-sensitive.
func (cl *Clan) MemberByName(name string) (Member, bool) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.memberByNameLocked(name)
}

func (cl *Clan) memberByNameLocked(name string) (Member, bool) {
	for _, m := range cl.members {
		if m.Name == name {
			return *m, true
		}
	}
	return Member{}, false
}

// Members returns the roster in ascending object id order.
func (cl *Clan) Members() []Member {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.membersLocked()
}

func (cl *Clan) membersLocked() []Member {
	out := make([]Member, 0, len(cl.members))
	for _, m := range cl.members {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ObjectID < out[j].ObjectID })
	return out
}

// OnlineMemberIDs returns the object ids of the members online, in
// ascending order.
func (cl *Clan) OnlineMemberIDs() []int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	var out []int32
	for id, m := range cl.members {
		if m.Online {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// MembersCount is the roster size, every sub-unit included.
func (cl *Clan) MembersCount() int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return len(cl.members)
}

func (cl *Clan) subunitCountLocked(pledgeType int) int {
	n := 0
	for _, m := range cl.members {
		if m.PledgeType == pledgeType {
			n++
		}
	}
	return n
}

// MaxMembers is how many members the sub-unit pledgeType may hold at the
// clan's level; 0 for an unknown sub-unit.
func MaxMembers(level, pledgeType int) int {
	switch pledgeType {
	case SubunitMain:
		switch level {
		case 0:
			return 10
		case 1:
			return 15
		case 2:
			return 20
		case 3:
			return 30
		default:
			return 40
		}
	case SubunitAcademy, SubunitRoyal1, SubunitRoyal2:
		return 20
	case SubunitKnight1, SubunitKnight2, SubunitKnight3, SubunitKnight4:
		return 10
	}
	return 0
}

// RankPrivileges returns the privilege mask of power grade rank; none when
// the clan never set it.
func (cl *Clan) RankPrivileges(rank int) int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.privileges[rank]
}

// MemberPrivileges returns the privilege mask objectID holds: every
// privilege for the leader, its rank's otherwise, none for a non-member.
func (cl *Clan) MemberPrivileges(objectID int32) int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.memberPrivilegesLocked(objectID)
}

func (cl *Clan) memberPrivilegesLocked(objectID int32) int32 {
	if objectID == cl.leaderID {
		return int32(PrivAll)
	}
	m, ok := cl.members[objectID]
	if !ok {
		return int32(PrivNone)
	}
	return cl.privileges[m.PowerGrade]
}

// HasPrivilege reports whether objectID holds p.
func (cl *Clan) HasPrivilege(objectID int32, p Privilege) bool {
	return cl.MemberPrivileges(objectID)&int32(p) != 0
}

// leadsSubunit returns the sub-unit objectID leads, 0 when none.
func (cl *Clan) leadsSubunit(objectID int32) int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.leadsSubunitLocked(objectID)
}

// PowerGradeCounts returns how many members hold each power grade 0-9.
func (cl *Clan) PowerGradeCounts() [10]int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	var out [10]int
	for _, m := range cl.members {
		if m.PowerGrade >= 0 && m.PowerGrade < len(out) {
			out[m.PowerGrade]++
		}
	}
	return out
}

// SetOnline marks objectID online with its live values, reporting whether
// it is a member.
func (cl *Clan) SetOnline(objectID int32, live Member) bool {
	return cl.setPresence(objectID, live, true)
}

// SetOffline marks objectID offline, keeping its live values as the
// offline row, and reports whether it is a member.
func (cl *Clan) SetOffline(objectID int32, live Member) bool {
	return cl.setPresence(objectID, live, false)
}

func (cl *Clan) setPresence(objectID int32, live Member, online bool) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	m, ok := cl.members[objectID]
	if !ok {
		return false
	}
	m.Title, m.Level, m.ClassID, m.Sex, m.Race = live.Title, live.Level, live.ClassID, live.Sex, live.Race
	m.Online = online
	return true
}

// RefreshLevel records objectID's new level, reporting whether it is a
// member.
func (cl *Clan) RefreshLevel(objectID int32, level int) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	m, ok := cl.members[objectID]
	if ok {
		m.Level = level
	}
	return ok
}

func (cl *Clan) rowLocked() Row {
	return Row{
		ID: cl.id, Name: cl.name, Level: cl.level, Reputation: cl.reputation,
		CastleID: cl.castleID, AllyID: cl.allyID, AllyName: cl.allyName,
		LeaderID: cl.leaderID, NewLeaderID: cl.newLeaderID,
		CrestID: cl.crestID, CrestLargeID: cl.crestLargeID, AllyCrestID: cl.allyCrestID,
		AllyPenaltyExpiry: cl.allyPenaltyExpiry, AllyPenaltyType: cl.allyPenaltyType,
		CharPenaltyExpiry: cl.charPenaltyExpiry, DissolvingExpiry: cl.dissolvingExpiry,
	}
}
