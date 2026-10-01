package clan

import (
	"context"
	"sync"
	"time"
	"unicode"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/rs/zerolog"
)

// writeTimeout bounds one clan row write.
const writeTimeout = 2 * time.Second

// Config is the clans.properties knobs the clan core reads.
type Config struct {
	// JoinDays is DaysBeforeJoinAClan: how long a member that left or was
	// expelled waits to join again, and a clan that expelled waits to
	// recruit.
	JoinDays int
	// CreateDays is DaysBeforeCreateAClan: how long a leader that left its
	// clan waits to found another.
	CreateDays int
	// MembersForWar is ClanMembersForWar: how many members a clan needs to
	// declare war, or to be declared war on.
	MembersForWar int
	// WarPenaltyDays is ClanWarPenaltyWhenEnded: how long a clan that
	// stopped a war waits to declare it again; 0 forgets the war at once.
	WarPenaltyDays int
	// LifeCrystalNeeded is players.properties LifeCrystalNeeded: learning a
	// clan skill also takes one of the skill's item from the leader.
	LifeCrystalNeeded bool
	// MembersCanWithdrawFromWarehouse is MembersCanWithdrawFromClanWH: a
	// member holding the warehouse-search privilege may withdraw from the
	// clan warehouse, not only the leader.
	MembersCanWithdrawFromWarehouse bool
	// AllyJoinDaysWhenLeft is DaysBeforeJoinAllyWhenLeaved: how long a
	// clan that left its alliance waits to join one again.
	AllyJoinDaysWhenLeft int
	// AllyJoinDaysWhenDismissed is DaysBeforeJoinAllyWhenDismissed: how
	// long a clan dismissed from its alliance waits to join one again.
	AllyJoinDaysWhenDismissed int
	// AcceptClanDaysWhenDismissed is DaysBeforeAcceptNewClanWhenDismissed:
	// how long an alliance leader that dismissed a clan waits to invite
	// another.
	AcceptClanDaysWhenDismissed int
	// CreateAllyDaysWhenDissolved is DaysBeforeCreateNewAllyWhenDissolved:
	// how long a clan that dissolved its alliance waits to found another.
	CreateAllyDaysWhenDissolved int
	// MaxClansInAlly is MaxNumOfClansInAlly: how many clans an alliance
	// holds, its leading clan included.
	MaxClansInAlly int
}

// DefaultConfig is the shipped clans.properties and LifeCrystalNeeded.
func DefaultConfig() Config {
	return Config{
		JoinDays: 1, CreateDays: 10, MembersForWar: 15, WarPenaltyDays: 5, LifeCrystalNeeded: true,
		AllyJoinDaysWhenLeft: 1, AllyJoinDaysWhenDismissed: 1, AcceptClanDaysWhenDismissed: 1,
		CreateAllyDaysWhenDissolved: 10, MaxClansInAlly: 3,
	}
}

// IDAllocator hands out object ids; a new clan's id comes from the same
// space as every other persisted object's.
type IDAllocator interface {
	NextID() (int32, error)
}

// Service runs the clan rules against the registry and queues the rows
// they change.
type Service struct {
	table   *Table
	store   Store
	writes  Writer
	ids     IDAllocator
	cfg     Config
	log     zerolog.Logger
	invites *Invites
	// subunitMu serializes sub-unit foundings, whose names are unique
	// across every clan.
	subunitMu sync.Mutex
	// allyMu serializes every alliance change: the clans of an alliance
	// and the alliance names stay fixed while it is held. It is taken
	// before any clan's mu.
	allyMu sync.Mutex
}

// NewService returns a Service over table. Writes go to store through
// writes, one lane per clan or character.
func NewService(table *Table, store Store, writes Writer, ids IDAllocator, cfg Config, now func() time.Time, log zerolog.Logger) *Service {
	if table == nil {
		table = NewTable()
	}
	return &Service{table: table, store: store, writes: writes, ids: ids, cfg: cfg, log: log, invites: NewInvites(now)}
}

// Table is the registry the service runs on.
func (s *Service) Table() *Table { return s.table }

// Invites is the pending clan invitation book.
func (s *Service) Invites() *Invites { return s.invites }

// write queues fn on ownerID's lane. Every caller holds the changed clan's
// mu, so the jobs on one lane follow the order the changes were made in and
// an older row snapshot never lands after a newer one. Enqueue only appends
// to the lane, so holding mu across it blocks nothing.
func (s *Service) write(ownerID int32, what string, fn func(context.Context, Store) error) {
	if s.store == nil {
		return
	}
	store, log := s.store, s.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("owner_id", ownerID).Msg("clan: " + what)
		}
	}
	if s.writes == nil {
		job()
		return
	}
	if !s.writes.Enqueue(ownerID, job) {
		log.Error().Int32("owner_id", ownerID).Msg("clan: " + what + ": write dropped")
	}
}

// updateClanLocked queues cl's clan_data row as it stands; cl.mu is held.
func (s *Service) updateClanLocked(cl *Clan) {
	row := cl.rowLocked()
	s.write(cl.id, "update clan", func(ctx context.Context, st Store) error { return st.UpdateClan(ctx, row) })
}

// ClanOf returns the clan c belongs to.
func (s *Service) ClanOf(c *player.Character) (*Clan, bool) {
	cl, ok := s.table.Get(c.ClanID())
	if !ok || !cl.IsMember(c.ID) {
		return nil, false
	}
	return cl, true
}

// RestoreMembership settles c's clan state as its row is loaded at
// selection: a clan that no longer exists, or no longer lists c, leaves it
// clanless; a penalty already over is cleared; the pledge class follows
// the clan.
func (s *Service) RestoreMembership(c *player.Character, now time.Time) {
	nowMs := now.UnixMilli()
	if c.ClanJoinExpiryTime() < nowMs {
		c.SetClanJoinExpiryTime(0)
	}
	if c.ClanCreateExpiryTime() < nowMs {
		c.SetClanCreateExpiryTime(0)
	}
	cl, ok := s.table.Get(c.ClanID())
	if !ok {
		c.SetClanID(0)
		return
	}
	if !cl.IsMember(c.ID) {
		c.SetClanID(0)
	}
	c.SetPledgeClass(s.pledgeClass(c))
}

// pledgeClass computes c's clan rank from its current clan membership.
// Nobility is not modeled yet (#218), so only heroism lifts it.
func (s *Service) pledgeClass(c *player.Character) int {
	cl, ok := s.ClanOf(c)
	class := 0
	if ok {
		m, _ := cl.Member(c.ID)
		class = PledgeClass(cl.Level(), cl.IsLeader(c.ID), m.PledgeType, cl.leadsSubunit(c.ID))
	}
	if c.IsHero() && class < 8 {
		class = 8
	}
	return class
}

// PledgeClass is the clan rank of a member of a clan at level, leading it
// or not, in sub-unit pledgeType and leading sub-unit leads (0 for none).
func PledgeClass(level int, leader bool, pledgeType, leads int) int {
	// Per clan level 6-8: the academy, royal guard and knight ranks, then
	// the main clan's leader, royal captain, knight captain and plain ranks.
	type ranks struct{ academy, royal, knight, leader, royalLead, knightLead, main int }
	table := map[int]ranks{
		6: {1, 2, 0, 5, 4, 3, 3},
		7: {1, 3, 2, 7, 6, 5, 4},
		8: {1, 4, 3, 8, 7, 6, 5},
	}
	switch level {
	case 4:
		if leader {
			return 3
		}
		return 0
	case 5:
		if leader {
			return 4
		}
		return 2
	case 6, 7, 8:
		r := table[level]
		switch {
		case pledgeType == SubunitAcademy:
			return r.academy
		case isRoyal(pledgeType):
			return r.royal
		case isKnight(pledgeType):
			return r.knight
		case pledgeType != SubunitMain:
			return 0
		case leader:
			return r.leader
		case isRoyal(leads):
			return r.royalLead
		case isKnight(leads):
			return r.knightLead
		default:
			return r.main
		}
	default:
		return 1
	}
}

func isRoyal(pledgeType int) bool {
	return pledgeType == SubunitRoyal1 || pledgeType == SubunitRoyal2
}

func isKnight(pledgeType int) bool {
	switch pledgeType {
	case SubunitKnight1, SubunitKnight2, SubunitKnight3, SubunitKnight4:
		return true
	}
	return false
}

// validName reports whether name is made of letters and digits only, as a
// clan name must be.
func validName(name string) bool {
	for _, r := range name {
		if r > 0xFFFF || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// nameLength is name's length in UTF-16 units.
func nameLength(name string) int {
	n := 0
	for _, r := range name {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}
