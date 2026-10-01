package clan

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// dayMillis is one day in epoch milliseconds, the unit the clan penalties
// are configured in.
const dayMillis = int64(24 * time.Hour / time.Millisecond)

// minReputationLevel is the clan level below which a clan neither gains nor
// loses reputation.
const minReputationLevel = 5

// Reputation bounds.
const (
	minReputation = -100000000
	maxReputation = 100000000
)

// ladderSize is how many clans the reputation ladder ranks.
const ladderSize = 99

// Row is one clan_data row.
type Row struct {
	ID                int32
	Name              string
	Level             int
	Reputation        int
	CastleID          int32
	AllyID            int32
	AllyName          string
	LeaderID          int32
	NewLeaderID       int32
	CrestID           int32
	CrestLargeID      int32
	AllyCrestID       int32
	AllyPenaltyExpiry int64
	AllyPenaltyType   int
	CharPenaltyExpiry int64
	DissolvingExpiry  int64
}

// MemberRow is one characters row of a clan member.
type MemberRow struct {
	ClanID int32
	Member
}

// PrivilegeRow is one clan_privs row.
type PrivilegeRow struct {
	ClanID int32
	Rank   int
	Privs  int32
}

// Snapshot is everything the clan registry restores from at boot.
type Snapshot struct {
	Clans      []Row
	Members    []MemberRow
	Privileges []PrivilegeRow
}

// Table is the clan registry. mu guards clans; each clan guards itself.
type Table struct {
	mu    sync.RWMutex
	clans map[int32]*Clan
}

// NewTable returns an empty registry.
func NewTable() *Table { return &Table{clans: map[int32]*Clan{}} }

// Restore fills the registry from the stored rows as of now, once at boot
// before any player connects. A clan below level 5 holds no reputation,
// whatever its row says; a member penalty survives while it is less than
// joinDays past its start; an alliance penalty survives until it ends. The
// ladder then ranks the 99 clans with the most positive reputation.
func (t *Table) Restore(s Snapshot, now time.Time, joinDays int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	nowMs := now.UnixMilli()
	for _, r := range s.Clans {
		cl := &Clan{
			id: r.ID, name: r.Name, leaderID: r.LeaderID, newLeaderID: r.NewLeaderID,
			level: r.Level, castleID: r.CastleID,
			crestID: r.CrestID, crestLargeID: r.CrestLargeID,
			allyID: r.AllyID, allyName: r.AllyName, allyCrestID: r.AllyCrestID,
			dissolvingExpiry: r.DissolvingExpiry,
			members:          map[int32]*Member{},
			privileges:       map[int]int32{},
		}
		if r.AllyPenaltyExpiry > nowMs {
			cl.allyPenaltyExpiry, cl.allyPenaltyType = r.AllyPenaltyExpiry, r.AllyPenaltyType
		}
		if r.CharPenaltyExpiry+int64(joinDays)*dayMillis > nowMs {
			cl.charPenaltyExpiry = r.CharPenaltyExpiry
		}
		if cl.level >= minReputationLevel {
			cl.reputation = clampReputation(r.Reputation)
		}
		t.clans[r.ID] = cl
	}
	for _, m := range s.Members {
		if cl, ok := t.clans[m.ClanID]; ok {
			member := m.Member
			member.Online = false
			cl.members[member.ObjectID] = &member
		}
	}
	for _, p := range s.Privileges {
		if cl, ok := t.clans[p.ClanID]; ok {
			cl.privileges[p.Rank] = p.Privs
		}
	}
	t.rankLadder(s.Clans)
}

// rankLadder gives ranks 1, 2, ... to the clans with positive reputation,
// best stored score first, among the 99 best stored scores.
func (t *Table) rankLadder(rows []Row) {
	ordered := append([]Row(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Reputation > ordered[j].Reputation })
	if len(ordered) > ladderSize {
		ordered = ordered[:ladderSize]
	}
	rank := 1
	for _, r := range ordered {
		if cl := t.clans[r.ID]; cl != nil && cl.reputation > 0 {
			cl.rank = rank
			rank++
		}
	}
}

func clampReputation(v int) int {
	return min(max(v, minReputation), maxReputation)
}

// Get returns the clan with id.
func (t *Table) Get(id int32) (*Clan, bool) {
	if t == nil || id == 0 {
		return nil, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	cl, ok := t.clans[id]
	return cl, ok
}

// ByName returns the clan named name, ignoring case.
func (t *Table) ByName(name string) (*Clan, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.byNameLocked(name)
}

func (t *Table) byNameLocked(name string) (*Clan, bool) {
	for _, cl := range t.clans {
		if strings.EqualFold(cl.Name(), name) {
			return cl, true
		}
	}
	return nil, false
}

// MemberClan returns the clan objectID belongs to.
func (t *Table) MemberClan(objectID int32) (*Clan, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, cl := range t.clans {
		if cl.IsMember(objectID) {
			return cl, true
		}
	}
	return nil, false
}

// Len is how many clans exist.
func (t *Table) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.clans)
}

// insert adds cl unless a clan already carries its name, ignoring case.
func (t *Table) insert(cl *Clan) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, taken := t.byNameLocked(cl.name); taken {
		return false
	}
	t.clans[cl.id] = cl
	return true
}
