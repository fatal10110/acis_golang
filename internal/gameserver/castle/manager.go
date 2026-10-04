package castle

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/rs/zerolog"
)

// writeTimeout bounds one castle write.
const writeTimeout = 2 * time.Second

// Finances is the castle row's money and tax columns, written together.
type Finances struct {
	Treasury, TaxRevenue, SeedIncome  int64
	CurrentTaxPercent, NextTaxPercent int
}

// Store writes the castle rows and the rows a change of owner touches.
type Store interface {
	UpdateTreasury(ctx context.Context, castleID int32, treasury int64) error
	UpdateTaxRevenue(ctx context.Context, castleID int32, revenue int64) error
	UpdateSeedIncome(ctx context.Context, castleID int32, income int64) error
	UpdateCertificates(ctx context.Context, castleID int32, n int) error
	UpdateCurrentTax(ctx context.Context, castleID int32, percent int) error
	UpdateNextTax(ctx context.Context, castleID int32, percent int) error
	UpdateFinances(ctx context.Context, castleID int32, f Finances) error
	// UpdateOwner clears the castle from every clan_data row holding it,
	// then gives it to clanID's row; 0 gives it to none.
	UpdateOwner(ctx context.Context, castleID, clanID int32) error
	// UnequipCirclets moves an offline character's equipped circlet
	// circletID and Lord's Crown back to its inventory.
	UnequipCirclets(ctx context.Context, circletID, ownerID int32) error
}

// Writer runs a write job on ownerID's lane, in the order queued.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Clans resolves a clan by id.
type Clans interface {
	Get(id int32) (*clan.Clan, bool)
}

// Owner is one clan_data row holding a castle.
type Owner struct {
	ClanID, CastleID int32
}

// Manager holds every castle's live state.
//
// owners serializes the changes of owner, so a clan never ends up owning
// two castles.
type Manager struct {
	byID  map[int]*Castle
	order []*Castle
	data  *castledata.Table
	clans Clans

	store  Store
	writes Writer
	log    zerolog.Logger

	owners sync.Mutex
}

// NewManager builds the castles of data, each free with nothing stored;
// Restore then sets them from the database. A nil store writes nothing; a
// nil writes runs each write on the caller.
func NewManager(data *castledata.Table, clans Clans, store Store, writes Writer, log zerolog.Logger) *Manager {
	m := &Manager{byID: map[int]*Castle{}, data: data, clans: clans, store: store, writes: writes, log: log}
	for _, d := range data.All() {
		c := &Castle{Castle: d, m: m}
		m.byID[d.ID] = c
		m.order = append(m.order, c)
	}
	for _, c := range m.order {
		c.parent = m.byID[c.ParentID]
	}
	return m
}

// Restore sets each castle from its stored row and gives it its owner,
// once at boot before any player connects. A row of a castle not loaded is
// skipped, and so is an owner row whose clan is not in clans; when two
// rows name one castle, the later one wins.
func (m *Manager) Restore(rows []Row, owners []Owner) {
	for _, r := range rows {
		if c, ok := m.byID[int(r.ID)]; ok {
			c.restore(r)
		}
	}
	for _, o := range owners {
		c, ok := m.byID[int(o.CastleID)]
		if !ok || o.ClanID <= 0 {
			continue
		}
		if _, ok := m.clans.Get(o.ClanID); !ok {
			continue
		}
		c.mu.Lock()
		c.ownerID = o.ClanID
		c.mu.Unlock()
	}
}

// Get returns the castle with id.
func (m *Manager) Get(id int) (*Castle, bool) {
	if m == nil {
		return nil, false
	}
	c, ok := m.byID[id]
	return c, ok
}

// ByAlias returns the castle with alias, matched case-insensitively.
func (m *Manager) ByAlias(alias string) (*Castle, bool) {
	if m == nil {
		return nil, false
	}
	d, ok := m.data.ByAlias(alias)
	if !ok {
		return nil, false
	}
	return m.Get(d.ID)
}

// ByOwner returns the castle clanID owns.
func (m *Manager) ByOwner(clanID int32) (*Castle, bool) {
	if m == nil {
		return nil, false
	}
	for _, c := range m.order {
		if c.OwnerID() == clanID {
			return c, true
		}
	}
	return nil, false
}

// All returns the castles in castles.xml order.
func (m *Manager) All() []*Castle {
	if m == nil {
		return nil
	}
	return append([]*Castle(nil), m.order...)
}

// SetOwner gives c to cl, unless cl already owns a castle, which it
// reports false for. The clan that owned c loses it and is returned, nil
// for none; telling the clans is the caller's, as is dismounting the
// former owner's leader from a wyvern. Every clan_data row holding c is
// cleared and cl's given it.
func (m *Manager) SetOwner(c *Castle, cl *clan.Clan) (former *clan.Clan, ok bool) {
	m.owners.Lock()
	defer m.owners.Unlock()
	if cl.CastleID() > 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.ownerID; old > 0 && old != cl.ID() {
		if oc, found := m.clans.Get(old); found {
			oc.SetCastleID(0)
			former = oc
		}
	}
	c.ownerID = cl.ID()
	cl.SetCastleID(int32(c.ID))
	c.writeOwnerLocked()
	return former, true
}

// RemoveOwner takes c from the clan owning it and returns that clan,
// reporting false when c has no owner. When the owner id names no clan,
// nothing changes and the clan returned is nil. Otherwise the castle's
// finances are reset (treasury, revenue and income to 0, both tax rates to
// the default) and every clan_data row holding it is cleared. Telling the
// clan, and taking the castle's circlets off its members, is the caller's.
func (m *Manager) RemoveOwner(c *Castle) (*clan.Clan, bool) {
	m.owners.Lock()
	defer m.owners.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ownerID <= 0 {
		return nil, false
	}
	cl, found := m.clans.Get(c.ownerID)
	if !found {
		return nil, true
	}
	cl.SetCastleID(0)
	c.ownerID = 0
	c.resetFinancesLocked()
	c.writeOwnerLocked()
	return cl, true
}

// UnequipCirclets queues the move of an offline member's equipped circlet
// of c and Lord's Crown back to its inventory, on the member's lane.
func (m *Manager) UnequipCirclets(c *Castle, memberID int32) {
	circlet := int32(c.CircletID)
	m.write(memberID, "unequip circlets", func(ctx context.Context, st Store) error {
		return st.UnequipCirclets(ctx, circlet, memberID)
	})
}

// writeOwnerLocked stores c's owner; c.mu is held.
func (c *Castle) writeOwnerLocked() {
	owner := c.ownerID
	c.write("update owner", func(ctx context.Context, st Store) error {
		return st.UpdateOwner(ctx, c.id(), owner)
	})
}

// write runs fn against the store on ownerID's lane, logging a failure.
func (m *Manager) write(ownerID int32, what string, fn func(context.Context, Store) error) {
	if m.store == nil {
		return
	}
	store, log := m.store, m.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("owner_id", ownerID).Msg("castle: " + what)
		}
	}
	if m.writes == nil {
		job()
		return
	}
	if !m.writes.Enqueue(ownerID, job) {
		log.Error().Int32("owner_id", ownerID).Msg("castle: " + what + ": write dropped")
	}
}
