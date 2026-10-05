// Package castlemanor runs the castles' manor: the seeds each castle sells
// and the crops it buys over the running manor period, the lists its owner
// sets for the next period, and the daily cycle that turns one period into
// the next.
//
// The cycle runs through three modes. The owner sets the next period's
// lists while the manor is modifiable; at the approve time it is approved;
// at the refresh time it goes under maintenance and the period rolls over:
// each owning clan's warehouse receives the crops the castle bought, the
// money set aside for crops nobody sold goes back to the castle as seed
// income, and the next period's lists become the running ones. Once the
// maintenance minutes have passed the owners' clan leaders are told the
// manor was updated and it is modifiable again. The lists persist in
// castle_manor_production and castle_manor_procure.
package castlemanor

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// TaskTimeout bounds one database read or write.
const TaskTimeout = 10 * time.Second

// writeLane is the persistence owner every manor save is queued under. No
// character, item or clan has object id 0.
const writeLane int32 = 0

// Status is the manor's mode.
type Status uint8

const (
	// StatusDisabled: the manor is off (AllowManor = False).
	StatusDisabled Status = iota
	// StatusModifiable: owners may set the next period's lists.
	StatusModifiable
	// StatusMaintenance: the period is rolling over; the manor NPCs turn
	// players away.
	StatusMaintenance
	// StatusApproved: the next period's lists are fixed.
	StatusApproved
)

// String is the mode's name as the admin manor page shows it.
func (s Status) String() string {
	switch s {
	case StatusModifiable:
		return "MODIFIABLE"
	case StatusMaintenance:
		return "MAINTENANCE"
	case StatusApproved:
		return "APPROVED"
	}
	return "DISABLED"
}

// Config is the manor's configuration.
type Config struct {
	// Enabled runs the manor (server.properties AllowManor).
	Enabled bool
	// CropRate multiplies the seed and crop limits (RateDropManor).
	CropRate int
	// RefreshHour and RefreshMin are the time of day the period rolls
	// over (clans.properties ManorRefreshTime, ManorRefreshMin).
	RefreshHour, RefreshMin int
	// ApproveHour and ApproveMin are the time of day the next period's
	// lists are approved (ManorApproveTime, ManorApproveMin).
	ApproveHour, ApproveMin int
	// MaintenanceMin is how many minutes the maintenance lasts
	// (ManorMaintenanceMin).
	MaintenanceMin int
	// SavePeriod is how often the lists are saved (ManorSavePeriodRate,
	// in hours); 0 saves them only on a rollover and on shutdown.
	SavePeriod time.Duration
}

// DefaultConfig returns the configuration used when no setting overrides
// it.
func DefaultConfig() Config {
	return Config{
		Enabled: true, CropRate: 1,
		RefreshHour: 20, ApproveHour: 6, MaintenanceMin: 6,
		SavePeriod: 2 * time.Hour,
	}
}

// ProductionRow is a castle_manor_production row.
type ProductionRow struct {
	CastleID, SeedID, Amount, StartAmount, Price int32
	NextPeriod                                   bool
}

// ProcureRow is a castle_manor_procure row.
type ProcureRow struct {
	CastleID, CropID, Amount, StartAmount, Price, RewardType int32
	NextPeriod                                               bool
}

// Store persists every castle's period lists.
type Store interface {
	// Load returns every stored row, each table in castle, item and period
	// order.
	Load(ctx context.Context) ([]ProductionRow, []ProcureRow, error)
	// Save replaces every stored row with production and procure.
	Save(ctx context.Context, production []ProductionRow, procure []ProcureRow) error
}

// Writer runs a database job later, on ownerID's persistence lane, so the
// manor never waits on the database.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Clans resolves the clans owning the castles.
type Clans interface {
	Get(id int32) (*clan.Clan, bool)
}

// Effects acts on the clans and players a rollover reaches. Its calls never
// call back into the Manager.
type Effects interface {
	// AddToClanWarehouse adds count of itemID to clan clanID's warehouse.
	AddToClanWarehouse(clanID, itemID int32, count int)
	// TellManorUpdated tells the player leaderID, when in the world, that
	// the manor information was updated.
	TellManorUpdated(leaderID int32)
}

// Option adjusts a Manager.
type Option func(*Manager)

// WithRoll rolls with roll, which returns an int in [0, n). The default
// rolls at random.
func WithRoll(roll func(n int) int) Option {
	return func(m *Manager) { m.roll = roll }
}

// periods is one castle's lists: the running period's and the next one's.
// A list is replaced, never changed in place; a Production in it may be in
// both periods' lists at once.
type periods struct {
	production, productionNext []*Production
	procure, procureNext       []*Procure
}

func (p *periods) lists(next bool) ([]*Production, []*Procure) {
	if next {
		return p.productionNext, p.procureNext
	}
	return p.production, p.procure
}

// Manager holds every castle's manor lists and runs the manor cycle. Every
// method is safe from any goroutine.
type Manager struct {
	cfg     Config
	seeds   *manor.Table
	castles *castle.Manager
	clans   Clans
	store   Store
	writes  Writer
	log     zerolog.Logger
	roll    func(n int) int

	// saveMu serializes saves, each of which writes the lists as they are
	// when it runs, so a later save never lands older lists.
	saveMu sync.Mutex

	// mu guards the fields below it; never held across I/O or an Effects
	// call.
	mu         sync.Mutex
	status     Status
	nextChange time.Time
	byCastle   map[int]*periods
	queue      *sim.Queue
	effects    Effects
	stopped    bool
}

// New returns the manor of castles over the seed table seeds, every castle
// with empty lists, persisting through store with a save queued on writes.
// Restore then Start bring it up. A disabled manor holds no lists, stays
// StatusDisabled and never saves.
func New(cfg Config, seeds *manor.Table, castles *castle.Manager, clans Clans, store Store, writes Writer, log zerolog.Logger, opts ...Option) *Manager {
	m := &Manager{
		cfg: cfg, seeds: seeds, castles: castles, clans: clans, store: store, writes: writes, log: log,
		roll: rnd.Get, status: StatusDisabled, byCastle: map[int]*periods{},
	}
	for _, opt := range opts {
		opt(m)
	}
	if cfg.Enabled {
		m.status = StatusApproved
		for _, c := range castles.All() {
			m.byCastle[c.ID] = &periods{}
		}
	}
	return m
}

// Enabled reports whether the manor runs.
func (m *Manager) Enabled() bool { return m != nil && m.cfg.Enabled }

// Seeds is the manor's seed table.
func (m *Manager) Seeds() *manor.Table {
	if m == nil {
		return nil
	}
	return m.seeds
}

// SeedsLimit is the most seeds of s a castle may sell over a period: its
// manors.xml limit times the crop rate.
func (m *Manager) SeedsLimit(s manor.Seed) int32 { return int32(s.SeedsLimit) * int32(m.cfg.CropRate) }

// CropsLimit is the most crops of s a castle may buy over a period: its
// manors.xml limit times the crop rate.
func (m *Manager) CropsLimit(s manor.Seed) int32 { return int32(s.CropsLimit) * int32(m.cfg.CropRate) }

// Restore loads the stored lists, before Start. A row of a castle not
// loaded is left out.
func (m *Manager) Restore(ctx context.Context) error {
	if !m.Enabled() || m.store == nil {
		return nil
	}
	production, procure, err := m.store.Load(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range production {
		p, ok := m.byCastle[int(r.CastleID)]
		if !ok {
			continue
		}
		sp := NewProduction(r.SeedID, r.Amount, r.Price, r.StartAmount)
		if r.NextPeriod {
			p.productionNext = append(p.productionNext, sp)
		} else {
			p.production = append(p.production, sp)
		}
	}
	for _, r := range procure {
		p, ok := m.byCastle[int(r.CastleID)]
		if !ok {
			continue
		}
		cp := NewProcure(r.CropID, r.Amount, r.Price, r.StartAmount, r.RewardType)
		if r.NextPeriod {
			p.procureNext = append(p.procureNext, cp)
		} else {
			p.procure = append(p.procure, cp)
		}
	}
	return nil
}

// Start runs the cycle on queue, which it owns from then on, in the time
// zone of the queue's clock: the mode is set from the time of day and the
// next change scheduled, and the lists are saved every SavePeriod. effects
// reaches the clans and players a change acts on.
func (m *Manager) Start(queue *sim.Queue, effects Effects) {
	if !m.Enabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue, m.effects = queue, effects
	now := queue.Now()
	m.status = statusAt(m.cfg, now)
	m.scheduleLocked(now)
	if m.cfg.SavePeriod > 0 {
		queue.Every(m.cfg.SavePeriod, m.queueSave)
	}
	m.log.Info().Stringer("mode", m.status).Time("next_change", m.nextChange).Msg("manor: started")
}

// Stop cancels the cycle, then saves the lists as they stand.
func (m *Manager) Stop(ctx context.Context) error {
	if !m.Enabled() {
		return nil
	}
	m.mu.Lock()
	m.stopped = true
	queue := m.queue
	m.mu.Unlock()
	if queue != nil {
		queue.Close()
	}
	return m.save(ctx)
}

// statusAt is the mode the manor starts in at now: modifiable from the end
// of the maintenance minute of any hour from the refresh hour on, before
// the approve hour, and in the approve hour up to the approve minute; under
// maintenance in the refresh hour from the refresh minute until the
// maintenance ends; approved otherwise. An approved start already past the
// refresh time of its day changes at once.
func statusAt(cfg Config, now time.Time) Status {
	hour, minute := now.Hour(), now.Minute()
	maintenanceEnd := cfg.RefreshMin + cfg.MaintenanceMin
	switch {
	case hour >= cfg.RefreshHour && minute >= maintenanceEnd,
		hour < cfg.ApproveHour,
		hour == cfg.ApproveHour && minute <= cfg.ApproveMin:
		return StatusModifiable
	case hour == cfg.RefreshHour && minute >= cfg.RefreshMin && minute < maintenanceEnd:
		return StatusMaintenance
	}
	return StatusApproved
}

// nextChangeAt is when a manor in status at now changes mode, at second 0
// of the minute with now's milliseconds: the approve time for a modifiable
// manor, the next day's when today's has passed; the end of today's
// maintenance for one under maintenance; today's refresh time for an
// approved one. A time already passed is due at once.
func nextChangeAt(cfg Config, status Status, now time.Time) time.Time {
	y, mo, d := now.Date()
	ms := now.Nanosecond() / int(time.Millisecond) * int(time.Millisecond)
	at := func(day, hour, minute int) time.Time {
		return time.Date(y, mo, day, hour, minute, 0, ms, now.Location())
	}
	switch status {
	case StatusModifiable:
		next := at(d, cfg.ApproveHour, cfg.ApproveMin)
		if next.Before(now) {
			next = at(d+1, cfg.ApproveHour, cfg.ApproveMin)
		}
		return next
	case StatusMaintenance:
		return at(d, cfg.RefreshHour, cfg.RefreshMin+cfg.MaintenanceMin)
	}
	return at(d, cfg.RefreshHour, cfg.RefreshMin)
}

// scheduleLocked schedules the next mode change. Runs under mu.
func (m *Manager) scheduleLocked(now time.Time) {
	m.nextChange = nextChangeAt(m.cfg, m.status, now)
	m.queue.After(max(m.nextChange.Sub(now), 0), m.changeMode)
}

// changeMode moves the manor to its next mode and schedules the change
// after it. An approved manor goes under maintenance and rolls the period
// over; a manor under maintenance becomes modifiable and tells each owning
// clan's leader; a modifiable manor is approved.
func (m *Manager) changeMode() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	var payouts []func()
	save := false
	switch m.status {
	case StatusApproved:
		m.status = StatusMaintenance
		payouts = m.rolloverLocked()
		save = true
	case StatusMaintenance:
		for _, c := range m.castles.All() {
			if owner, ok := m.ownerLocked(c); ok && m.effects != nil {
				leader, sink := owner.LeaderID(), m.effects
				payouts = append(payouts, func() { sink.TellManorUpdated(leader) })
			}
		}
		m.status = StatusModifiable
	case StatusModifiable:
		// Approving charges no castle anything: the next period's lists
		// stand as set.
		m.status = StatusApproved
	}
	m.scheduleLocked(m.queue.Now())
	status := m.status
	m.mu.Unlock()

	for _, pay := range payouts {
		pay()
	}
	if save {
		m.queueSave()
	}
	m.log.Debug().Stringer("mode", status).Msg("manor: mode changed")
}

// rolloverLocked turns each owned castle's next period into its running
// one and returns the payouts to make once mu is released. Runs under mu.
//
// Each crop the castle bought, at least one bought and not all wanted
// bought, pays 90% of the count bought, truncated, into the owning clan's
// warehouse as the crop's mature item; a count truncated to 0 pays 1 with a
// 90 in 99 chance. The money set aside for crops still wanted goes back to
// the castle as seed income. The new running lists are the next period's;
// the next period then starts over from the same lists, each amount back
// at its start amount, unless the treasury cannot cover the new running
// period's cost, which leaves the next period empty.
func (m *Manager) rolloverLocked() []func() {
	var out []func()
	sink := m.effects
	for _, c := range m.castles.All() {
		owner, ok := m.ownerLocked(c)
		if !ok {
			continue
		}
		p := m.byCastle[c.ID]
		if p == nil {
			continue
		}
		var refund int64
		for _, crop := range p.procure {
			if crop.StartAmount <= 0 {
				continue
			}
			left := crop.Amount()
			if crop.StartAmount != left {
				count := truncate(float64(crop.StartAmount-left) * 0.9)
				if count < 1 && m.roll(99) < 90 {
					count = 1
				}
				if seed, ok := m.seeds.SeedByCrop(int(crop.ID)); ok && count > 0 && sink != nil {
					clanID, itemID := owner.ID(), int32(seed.MatureID)
					out = append(out, func() { sink.AddToClanWarehouse(clanID, itemID, int(count)) })
				}
			}
			if left > 0 {
				refund += int64(left * crop.Price)
			}
		}
		out = append(out, func() { c.RiseSeedIncome(refund) })

		production, procure := p.productionNext, p.procureNext
		p.production, p.procure = production, procure
		if c.Treasury() < m.costLocked(p, false) {
			p.productionNext, p.procureNext = nil, nil
			continue
		}
		p.productionNext = slices.Clone(production)
		for _, sp := range p.productionNext {
			sp.SetAmount(sp.StartAmount)
		}
		p.procureNext = slices.Clone(procure)
		for _, cp := range p.procureNext {
			cp.SetAmount(cp.StartAmount)
		}
	}
	return out
}

// ownerLocked is the clan owning c. Runs under mu.
func (m *Manager) ownerLocked(c *castle.Castle) (*clan.Clan, bool) {
	id := c.OwnerID()
	if id <= 0 || m.clans == nil {
		return nil, false
	}
	return m.clans.Get(id)
}

// Status is the manor's mode.
func (m *Manager) Status() Status {
	if m == nil {
		return StatusDisabled
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// IsUnderMaintenance reports whether the period is rolling over.
func (m *Manager) IsUnderMaintenance() bool { return m.Status() == StatusMaintenance }

// IsManorApproved reports whether the next period's lists are approved.
func (m *Manager) IsManorApproved() bool { return m.Status() == StatusApproved }

// IsModifiablePeriod reports whether owners may set the next period's
// lists.
func (m *Manager) IsModifiablePeriod() bool { return m.Status() == StatusModifiable }

// NextModeChange is when the mode next changes, the zero time before Start
// or for a disabled manor.
func (m *Manager) NextModeChange() time.Time {
	if m == nil {
		return time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nextChange
}

// SeedProduction returns castle castleID's seed list for the next period,
// or the running one.
func (m *Manager) SeedProduction(castleID int, next bool) []*Production {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byCastle[castleID]
	if !ok {
		return nil
	}
	production, _ := p.lists(next)
	return slices.Clone(production)
}

// SeedProduct returns seed seedID of castle castleID's seed list for the
// next period, or the running one.
func (m *Manager) SeedProduct(castleID int, seedID int32, next bool) (*Production, bool) {
	for _, sp := range m.SeedProduction(castleID, next) {
		if sp.ID == seedID {
			return sp, true
		}
	}
	return nil, false
}

// CropProcure returns castle castleID's crop list for the next period, or
// the running one.
func (m *Manager) CropProcure(castleID int, next bool) []*Procure {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byCastle[castleID]
	if !ok {
		return nil
	}
	_, procure := p.lists(next)
	return slices.Clone(procure)
}

// CropProcureOf returns crop cropID of castle castleID's crop list for the
// next period, or the running one.
func (m *Manager) CropProcureOf(castleID int, cropID int32, next bool) (*Procure, bool) {
	for _, cp := range m.CropProcure(castleID, next) {
		if cp.ID == cropID {
			return cp, true
		}
	}
	return nil, false
}

// SetNextSeedProduction makes list castle castleID's seed list for the next
// period.
func (m *Manager) SetNextSeedProduction(castleID int, list []*Production) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.byCastle[castleID]; ok {
		p.productionNext = slices.Clone(list)
	}
}

// SetNextCropProcure makes list castle castleID's crop list for the next
// period.
func (m *Manager) SetNextCropProcure(castleID int, list []*Procure) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.byCastle[castleID]; ok {
		p.procureNext = slices.Clone(list)
	}
}

// ManorCost is what castle castleID's lists for the next period, or the
// running one, cost: each seed's reference price, 1 for a seed the table
// does not hold, and each crop's price, times its start amount.
func (m *Manager) ManorCost(castleID int, next bool) int64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byCastle[castleID]
	if !ok {
		return 0
	}
	return m.costLocked(p, next)
}

// costLocked is ManorCost of p. Each product is a 32-bit product, wrapping
// on overflow. Runs under mu.
func (m *Manager) costLocked(p *periods, next bool) int64 {
	production, procure := p.lists(next)
	var total int64
	for _, sp := range production {
		s, ok := m.seeds.Seed(sp.ID)
		if !ok {
			total++
			continue
		}
		total += int64(s.SeedReferencePrice * sp.StartAmount)
	}
	for _, cp := range procure {
		total += int64(cp.Price * cp.StartAmount)
	}
	return total
}

// Reset empties castle castleID's lists, both periods', as the castle
// loses its owner. A disabled manor does nothing.
func (m *Manager) Reset(castleID int) {
	if !m.Enabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.byCastle[castleID]; ok {
		*p = periods{}
	}
}

// queueSave queues a save of the lists on the persistence lane. Never
// called under mu: a writer without lanes runs the save inline.
func (m *Manager) queueSave() {
	if m.store == nil {
		return
	}
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
		defer cancel()
		if err := m.save(ctx); err != nil {
			m.log.Error().Err(err).Msg("manor: save")
		}
	}
	if m.writes == nil {
		job()
		return
	}
	if !m.writes.Enqueue(writeLane, job) {
		m.log.Error().Msg("manor: save dropped")
	}
}

// save writes every castle's lists as they stand.
func (m *Manager) save(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	production, procure := m.snapshot()
	return m.store.Save(ctx, production, procure)
}

// snapshot returns every castle's lists as rows, castle by castle in
// castle order, the running period's before the next one's.
func (m *Manager) snapshot() ([]ProductionRow, []ProcureRow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var production []ProductionRow
	var procure []ProcureRow
	for _, c := range m.castles.All() {
		p, ok := m.byCastle[c.ID]
		if !ok {
			continue
		}
		id := int32(c.ID)
		for _, next := range []bool{false, true} {
			seeds, crops := p.lists(next)
			for _, sp := range seeds {
				production = append(production, ProductionRow{
					CastleID: id, SeedID: sp.ID, Amount: sp.Amount(), StartAmount: sp.StartAmount, Price: sp.Price, NextPeriod: next,
				})
			}
			for _, cp := range crops {
				procure = append(procure, ProcureRow{
					CastleID: id, CropID: cp.ID, Amount: cp.Amount(), StartAmount: cp.StartAmount, Price: cp.Price,
					RewardType: cp.Reward, NextPeriod: next,
				})
			}
		}
	}
	return production, procure
}

// truncate narrows v toward zero to an int32, saturating at its range.
func truncate(v float64) int32 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= math.MaxInt32:
		return math.MaxInt32
	case v <= math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}
