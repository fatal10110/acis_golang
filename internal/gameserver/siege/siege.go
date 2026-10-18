// Package siege runs the castle sieges: the clans registered on each side,
// the siege calendar, the siege itself, the battlefield it turns on, and
// the clans' siege kills and deaths.
//
// Other systems read a siege through a narrow surface: ActiveAt (the siege
// in progress whose battlefield holds a point) and the side reads at a
// point (AttackerAt, DefenderAt, SameSideAt), At (the siege whose
// battlefield holds a point), Get (a castle's siege), Siege.InProgress,
// Siege.Attacking, Siege.Side, Siege.CheckSides, Siege.OnOppositeSides,
// Engine.Registered, the clan rules' reads (Registration, Fighting,
// Opposed, StateOf) and the kill counters; Forget drops a destroyed clan.
package siege

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// Side is the side a clan is registered on.
type Side uint8

// The sides. SideNone is no registration. A pending clan asked to defend
// and waits for the castle lord's approval.
const (
	SideNone Side = iota
	SideOwner
	SideDefender
	SideAttacker
	SidePending
)

var sideNames = [...]string{"", "OWNER", "DEFENDER", "ATTACKER", "PENDING"}

// String is the side as the siege_clans type column spells it.
func (s Side) String() string {
	if int(s) < len(sideNames) {
		return sideNames[s]
	}
	return fmt.Sprintf("Side(%d)", uint8(s))
}

// ParseSide reads a siege_clans type.
func ParseSide(s string) (Side, bool) {
	for i, name := range sideNames {
		if i > 0 && name == s {
			return Side(i), true
		}
	}
	return SideNone, false
}

// Status is where a siege stands in its cycle.
type Status uint8

const (
	// StatusRegistrationOpened takes registrations: the state between two
	// sieges.
	StatusRegistrationOpened Status = iota
	// StatusRegistrationOver closes the registrations, the day before the
	// siege.
	StatusRegistrationOver
	// StatusInProgress is the siege being fought.
	StatusInProgress
)

// String names s as the admin castle list shows it.
func (s Status) String() string {
	switch s {
	case StatusRegistrationOpened:
		return "REGISTRATION_OPENED"
	case StatusRegistrationOver:
		return "REGISTRATION_OVER"
	case StatusInProgress:
		return "IN_PROGRESS"
	}
	return fmt.Sprintf("Status(%d)", uint8(s))
}

// State is a player's siege state: what UserInfo and the relations show
// of the side its clan fights on.
type State int32

const (
	StateNone     State = 0
	StateAttacker State = 1
	StateDefender State = 2
)

// Config is the siege.properties castle siege settings.
type Config struct {
	// Length is how long a siege lasts (SiegeLength, in minutes).
	Length time.Duration
	// MinClanLevel is the clan level a clan needs to register
	// (SiegeClanMinLevel).
	MinClanLevel int
	// MaxAttackers and MaxDefenders cap each side's registrations
	// (AttackerMaxClans, DefenderMaxClans); pending defenders count as
	// defenders.
	MaxAttackers, MaxDefenders int
	// AttackerRespawn is how long a dead attacker waits before it restarts
	// (AttackerRespawn, in milliseconds).
	AttackerRespawn time.Duration
}

// DefaultConfig is the siege configuration with every key at its default.
func DefaultConfig() Config {
	return Config{
		Length:          120 * time.Minute,
		MinClanLevel:    4,
		MaxAttackers:    10,
		MaxDefenders:    10,
		AttackerRespawn: 10 * time.Second,
	}
}

// ClanRow is one siege_clans row.
type ClanRow struct {
	CastleID, ClanID int32
	Side             Side
}

// Store persists the siege registrations.
type Store interface {
	LoadClans(ctx context.Context) ([]ClanRow, error)
	// SaveClan registers clanID with side, replacing an earlier side.
	SaveClan(ctx context.Context, castleID, clanID int32, side Side) error
	DeleteClan(ctx context.Context, castleID, clanID int32) error
	DeleteClans(ctx context.Context, castleID int32) error
	DeletePending(ctx context.Context, castleID int32) error
}

// Writer runs a write job on ownerID's lane, in the order queued.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Clans resolves the clans the sieges name.
type Clans interface {
	Get(id int32) (*clan.Clan, bool)
	// Allies lists the clans of alliance allyID.
	Allies(allyID int32) []*clan.Clan
}

// Notifier tells the world what the sieges do. Its calls are made with no
// siege lock held, so they may read the sieges back.
type Notifier interface {
	// Announce sends m to every player in the world.
	Announce(m Message)
	// PlaySound plays the sound file for every player in the world.
	PlaySound(file string)
	// TellClans sends m to the members in the world of each clan, in
	// order.
	TellClans(clans []*clan.Clan, m Message)
	// SetSiegeState gives each member in the world of the clans the siege
	// state s, then resends its UserInfo and its relations.
	SetSiegeState(clans []*clan.Clan, s State)
	// Reputation adds points, negative to take them, to cl's reputation
	// score and tells its members with m.
	Reputation(cl *clan.Clan, points int, m Message)
	// CastleTaken tells that owner took c from former at the end of a
	// siege: former's members lose the items c's owners wear, and each
	// noble of owner in the world records the castle in its diary.
	CastleTaken(c *castle.Castle, owner, former *clan.Clan)
	// GuardDoors makes s the siege of each door named in gates, and of
	// whatever door later stands under that door's id.
	GuardDoors(gates []string, s door.Siege)
	// CloseDoors closes each door named in gates that stands open.
	CloseDoors(gates []string)
	// RespawnDoors stands each door named in gates again: a broken one
	// revives, each closes, and its HP is set to its maximum, or to half
	// of it when weak.
	RespawnDoors(gates []string, weak bool)
	// SpawnGuards spawns c's siege guards: with an owner, the NPCs of its
	// pc_siege_warfare_start makers, then a mercenary at each ticket on
	// its grounds, the tickets leaving the world; with none, the NPCs of
	// its siege_warfare_start makers.
	SpawnGuards(c *castle.Castle)
	// DespawnGuards deletes c's mercenaries, then the NPCs of its
	// pc_siege_warfare_start and siege_warfare_start makers.
	DespawnGuards(c *castle.Castle)
}

// writeTimeout bounds one siege write.
const writeTimeout = 2 * time.Second

// registration is one clan registered on a siege.
type registration struct {
	clanID int32
	side   Side
}

// Siege is one castle's siege.
//
// field is guarded by the engine's fieldMu, every other field by its mu.
type Siege struct {
	e      *Engine
	castle *castle.Castle
	field  *zone.Siege

	status Status
	// clans are the registered clans in the order they registered, the
	// owner first.
	clans []registration
	// formerOwner is the clan that held the castle when the siege started.
	formerOwner int32
	// endsAt is when the siege in progress ends, in Unix milliseconds.
	endsAt int64

	// start is the timer of the next step towards the siege's start and
	// clock the timer of its next countdown step; each gen counts the
	// timers armed, so a replaced timer's step does nothing.
	start    *sim.Timer
	startGen uint64
	clock    *sim.Timer
	clockGen uint64

	// engraved maps each of the castle's artifacts a clan engraved the
	// ruler on this siege to that clan's id.
	engraved map[int32]int32

	// pending are the life-cycle steps' world effects decided and not yet
	// run, in the order they were decided; draining is set while a
	// goroutine runs them (see runLifecycle).
	pending  effects
	draining bool

	// watchers are called with each status the siege changes to, in the
	// order they were added (see OnStatusChange).
	watchers []func(Status)
}

// counters are a clan's siege kills and deaths.
type counters struct {
	kills, deaths int
}

// Engine holds every castle's siege.
//
// mu guards every siege and the counters. It is taken before any castle
// or clan lock and never while holding one; nothing the engine calls
// while holding it calls back into the engine. The Notifier and the
// battlefields are called once mu is released. A castle gate's status
// update takes mu under the world doors' lock (ShowsDoorHP); the engine
// reaches the doors only through the Notifier, never holding mu.
type Engine struct {
	cfg     Config
	castles *castle.Manager
	clans   Clans
	store   Store
	writes  Writer
	log     zerolog.Logger

	mu sync.Mutex
	// fieldMu guards every siege's battlefield. It is held while a
	// battlefield is switched or swept, so a zone reload's Rebind never
	// lands between a siege reading its battlefield and acting on it. It is
	// taken after mu when both are held.
	fieldMu  sync.Mutex
	byID     map[int]*Siege
	order    []*Siege
	counters map[int32]*counters
	queue    *sim.Queue
	notify   Notifier
}

// New builds a siege for every castle of castles, each over the siege zone
// of fields whose residence is the castle (none when fields has none).
// Restore then loads the registrations. A nil store writes nothing; a nil
// writes runs each write on the caller.
func New(cfg Config, castles *castle.Manager, clans Clans, fields []*zone.Siege, store Store, writes Writer, log zerolog.Logger) *Engine {
	e := &Engine{
		cfg: cfg, castles: castles, clans: clans, store: store, writes: writes, log: log,
		byID: map[int]*Siege{}, counters: map[int32]*counters{},
	}
	for _, c := range castles.All() {
		s := &Siege{e: e, castle: c}
		for _, f := range fields {
			if f.ResidenceID == c.ID {
				s.field = f
				break
			}
		}
		e.byID[c.ID] = s
		e.order = append(e.order, s)
	}
	return e
}

// Restore registers each castle's owner as its siege's owner side, then
// the stored registrations, once at boot after the castles are restored
// and before Start. A row of a castle not loaded, or of a clan that does
// not exist, is skipped.
func (e *Engine) Restore(ctx context.Context) error {
	if e == nil {
		return nil
	}
	var rows []ClanRow
	if e.store != nil {
		var err error
		if rows, err = e.store.LoadClans(ctx); err != nil {
			return fmt.Errorf("restore sieges: %w", err)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.order {
		if owner := s.castle.OwnerID(); owner > 0 {
			if _, ok := e.clans.Get(owner); ok {
				s.setSideLocked(owner, SideOwner)
			}
		}
	}
	for _, r := range rows {
		s, ok := e.byID[int(r.CastleID)]
		if !ok {
			continue
		}
		if _, ok := e.clans.Get(r.ClanID); !ok {
			continue
		}
		s.setSideLocked(r.ClanID, r.Side)
	}
	return nil
}

// Start runs the sieges' calendars on queue from now on, telling the world
// through notify. Each siege first guards its castle's gates. A siege
// whose date has passed is given its next date, stored, and waits for it;
// any other siege takes its next step a second from now.
func (e *Engine) Start(queue *sim.Queue, notify Notifier) {
	if e == nil {
		return
	}
	if notify != nil {
		for _, s := range e.order {
			if gates := s.castle.Gates; len(gates) > 0 {
				notify.GuardDoors(gates, s)
			}
		}
	}
	e.mu.Lock()
	e.queue, e.notify = queue, notify
	e.mu.Unlock()
	for _, s := range e.order {
		s.runLifecycle(s.startAutoTaskLocked)
	}
}

// AttackerRespawn is how long a dead attacker of a siege in progress waits
// before its restart takes it away.
func (e *Engine) AttackerRespawn() time.Duration {
	if e == nil {
		return 0
	}
	return e.cfg.AttackerRespawn
}

// Get returns castleID's siege.
func (e *Engine) Get(castleID int) (*Siege, bool) {
	if e == nil {
		return nil, false
	}
	s, ok := e.byID[castleID]
	return s, ok
}

// All returns every siege, in castle order.
func (e *Engine) All() []*Siege {
	if e == nil {
		return nil
	}
	return slices.Clone(e.order)
}

// ActiveAt returns the siege in progress whose battlefield holds the point
// (x, y, z).
func (e *Engine) ActiveAt(x, y, z int) (*Siege, bool) {
	if e == nil {
		return nil, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.order {
		if f := s.Battlefield(); s.status == StatusInProgress && f != nil && f.ContainsPoint(x, y, z) {
			return s, true
		}
	}
	return nil, false
}

// AttackerAt reports whether clanID attacks the siege in progress whose
// battlefield holds the point (x, y, z).
func (e *Engine) AttackerAt(clanID int32, x, y, z int) bool {
	s, ok := e.ActiveAt(x, y, z)
	return ok && s.CheckSides(clanID, SideAttacker)
}

// DefenderAt reports whether clanID owns or defends the siege in progress
// whose battlefield holds the point (x, y, z).
func (e *Engine) DefenderAt(clanID int32, x, y, z int) bool {
	s, ok := e.ActiveAt(x, y, z)
	return ok && s.CheckSides(clanID, SideOwner, SideDefender)
}

// SameSideAt reports whether a siege is in progress whose battlefield holds
// the point (x, y, z), and the clans a and b are not on opposite sides of
// it. Two clans of which one is not registered, or no clan at all (0), are
// never on opposite sides.
func (e *Engine) SameSideAt(a, b int32, x, y, z int) bool {
	s, ok := e.ActiveAt(x, y, z)
	return ok && !s.OnOppositeSides(a, b)
}

// At returns the siege whose battlefield holds the point (x, y, z), being
// fought or not.
func (e *Engine) At(x, y, z int) (*Siege, bool) {
	if e == nil {
		return nil, false
	}
	for _, s := range e.order {
		if f := s.Battlefield(); f != nil && f.ContainsPoint(x, y, z) {
			return s, true
		}
	}
	return nil, false
}

// Registered reports whether clanID is registered, on any side, on any
// castle's siege.
func (e *Engine) Registered(clanID int32) bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.order {
		if s.sideLocked(clanID) != SideNone {
			return true
		}
	}
	return false
}

// RecordKill counts a siege kill for killerClanID and a siege death for
// victimClanID; 0 counts nothing for that side.
func (e *Engine) RecordKill(killerClanID, victimClanID int32) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if killerClanID != 0 {
		e.countersLocked(killerClanID).kills++
	}
	if victimClanID != 0 {
		e.countersLocked(victimClanID).deaths++
	}
}

// Kills is clanID's siege kills since the last siege it fought ended.
func (e *Engine) Kills(clanID int32) int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.counters[clanID]; ok {
		return c.kills
	}
	return 0
}

// Deaths is clanID's siege deaths since the last siege it fought ended.
func (e *Engine) Deaths(clanID int32) int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.counters[clanID]; ok {
		return c.deaths
	}
	return 0
}

func (e *Engine) countersLocked(clanID int32) *counters {
	c, ok := e.counters[clanID]
	if !ok {
		c = &counters{}
		e.counters[clanID] = c
	}
	return c
}

// nowLocked is the time on the clock the sieges run on.
func (e *Engine) nowLocked() time.Time {
	if e.queue == nil {
		return time.Now()
	}
	return e.queue.Now()
}

// Now is the time on the clock the sieges run on.
func (e *Engine) Now() time.Time {
	if e == nil {
		return time.Now()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nowLocked()
}

// Castle is the castle besieged.
func (s *Siege) Castle() *castle.Castle { return s.castle }

// Battlefield is the siege zone the siege turns on, nil for none.
func (s *Siege) Battlefield() *zone.Siege {
	s.e.fieldMu.Lock()
	defer s.e.fieldMu.Unlock()
	return s.field
}

// withBattlefield runs fn on the siege's battlefield, when it has one, with
// the battlefield held.
func (s *Siege) withBattlefield(fn func(f *zone.Siege)) {
	s.e.fieldMu.Lock()
	defer s.e.fieldMu.Unlock()
	if s.field != nil {
		fn(s.field)
	}
}

// Rebind moves each siege onto the battlefield of fields whose residence is
// its castle (none when fields has none), as a zone reload reads the
// battlefields anew: the new battlefield is on while the one it replaces
// is. Call it before fields hold anyone.
func (e *Engine) Rebind(fields []*zone.Siege) {
	if e == nil {
		return
	}
	e.fieldMu.Lock()
	defer e.fieldMu.Unlock()
	for _, s := range e.order {
		var next *zone.Siege
		for _, f := range fields {
			if f.ResidenceID == s.castle.ID {
				next = f
				break
			}
		}
		if next != nil && s.field != nil && s.field.Active() {
			next.SetActive(true)
		}
		s.field = next
	}
}

// Status is where the siege stands in its cycle.
func (s *Siege) Status() Status {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	return s.status
}

// InProgress reports whether the siege is being fought.
func (s *Siege) InProgress() bool { return s.Status() == StatusInProgress }

// Attacking reports whether the siege is being fought and clanID attacks
// it.
func (s *Siege) Attacking(clanID int32) bool {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	return s.status == StatusInProgress && s.sideLocked(clanID) == SideAttacker
}

// RegistrationOver reports whether the siege takes no more registrations.
func (s *Siege) RegistrationOver() bool { return s.Status() != StatusRegistrationOpened }

// Date is the siege date, in Unix milliseconds.
func (s *Siege) Date() int64 { return s.castle.SiegeDate() }

// Side is the side clanID is registered on, SideNone for none.
func (s *Siege) Side(clanID int32) Side {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	return s.sideLocked(clanID)
}

// CheckSides reports whether clanID is registered on one of sides, or on
// any side when sides is empty.
func (s *Siege) CheckSides(clanID int32, sides ...Side) bool {
	side := s.Side(clanID)
	if side == SideNone {
		return false
	}
	return len(sides) == 0 || slices.Contains(sides, side)
}

// OnOppositeSides reports whether one of the clans attacks and the other
// owns, defends or asks to defend.
func (s *Siege) OnOppositeSides(a, b int32) bool {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	sa, sb := s.sideLocked(a), s.sideLocked(b)
	if sa == SideNone || sb == SideNone {
		return false
	}
	return (sa == SideAttacker) != (sb == SideAttacker)
}

// Attackers returns the attacking clans, in registration order.
func (s *Siege) Attackers() []*clan.Clan {
	return s.clansOf(SideAttacker)
}

// Defenders returns the owner and the defending clans with their side, in
// registration order.
func (s *Siege) Defenders() []Registered {
	s.e.mu.Lock()
	ids := s.idsLocked(SideOwner, SideDefender)
	sides := make([]Side, len(ids))
	for i, id := range ids {
		sides[i] = s.sideLocked(id)
	}
	s.e.mu.Unlock()
	out := make([]Registered, 0, len(ids))
	for i, id := range ids {
		if cl, ok := s.e.clans.Get(id); ok {
			out = append(out, Registered{Clan: cl, Side: sides[i]})
		}
	}
	return out
}

// Pending returns the clans waiting for approval to defend, in
// registration order.
func (s *Siege) Pending() []*clan.Clan {
	return s.clansOf(SidePending)
}

// Registered is one clan registered on a siege, and its side.
type Registered struct {
	Clan *clan.Clan
	Side Side
}

func (s *Siege) clansOf(sides ...Side) []*clan.Clan {
	s.e.mu.Lock()
	ids := s.idsLocked(sides...)
	s.e.mu.Unlock()
	return s.e.resolve(ids)
}

func (e *Engine) resolve(ids []int32) []*clan.Clan {
	out := make([]*clan.Clan, 0, len(ids))
	for _, id := range ids {
		if cl, ok := e.clans.Get(id); ok {
			out = append(out, cl)
		}
	}
	return out
}

func (s *Siege) sideLocked(clanID int32) Side {
	if clanID == 0 {
		return SideNone
	}
	for _, r := range s.clans {
		if r.clanID == clanID {
			return r.side
		}
	}
	return SideNone
}

// idsLocked lists the clans registered on one of sides, in registration
// order.
func (s *Siege) idsLocked(sides ...Side) []int32 {
	var out []int32
	for _, r := range s.clans {
		if slices.Contains(sides, r.side) {
			out = append(out, r.clanID)
		}
	}
	return out
}

func (s *Siege) countLocked(sides ...Side) int {
	return len(s.idsLocked(sides...))
}

// setSideLocked registers clanID with side, keeping its place when it is
// registered already.
func (s *Siege) setSideLocked(clanID int32, side Side) {
	for i := range s.clans {
		if s.clans[i].clanID == clanID {
			s.clans[i].side = side
			return
		}
	}
	s.clans = append(s.clans, registration{clanID: clanID, side: side})
}

// removeLocked drops clanID's registration, reporting whether it had one.
func (s *Siege) removeLocked(clanID int32) bool {
	for i, r := range s.clans {
		if r.clanID == clanID {
			s.clans = slices.Delete(s.clans, i, i+1)
			return true
		}
	}
	return false
}

// write queues fn on the castle's persistence lane.
func (s *Siege) write(what string, fn func(context.Context, Store) error) {
	e := s.e
	if e.store == nil {
		return
	}
	store, log, id := e.store, e.log, int32(s.castle.ID)
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("castle_id", id).Msg("siege: " + what)
		}
	}
	if e.writes == nil {
		job()
		return
	}
	if !e.writes.Enqueue(id, job) {
		log.Error().Int32("castle_id", id).Msg("siege: " + what + ": write dropped")
	}
}

// effects are the calls a change makes once the engine's lock is
// released, in order.
type effects []func()

func (fx *effects) add(fn func()) { *fx = append(*fx, fn) }

func (fx effects) run() {
	for _, fn := range fx {
		fn()
	}
}

// runLifecycle decides one of the siege's life-cycle steps with decide,
// under the engine's lock, then runs the world effects it queued in the
// order the steps were decided: a step decided while another step's
// effects still run has its effects run after them, by the goroutine
// already running them, so its own call may return before they ran. A
// step's effects never run while the engine's lock is held.
func (s *Siege) runLifecycle(decide func(fx *effects)) {
	var fx effects
	s.e.mu.Lock()
	decide(&fx)
	s.pending = append(s.pending, fx...)
	if s.draining || len(s.pending) == 0 {
		s.e.mu.Unlock()
		return
	}
	s.draining = true
	s.e.mu.Unlock()
	s.drain()
}

// drain runs the pending life-cycle effects until none is left. A
// panicking effect drops the rest of its batch but still frees the queue,
// so the steps decided later run on the next step's call.
func (s *Siege) drain() {
	done := false
	defer func() {
		if !done {
			s.e.mu.Lock()
			s.draining = false
			s.e.mu.Unlock()
		}
	}()
	for {
		s.e.mu.Lock()
		batch := s.pending
		s.pending = nil
		if len(batch) == 0 {
			s.draining = false
			done = true
			s.e.mu.Unlock()
			return
		}
		s.e.mu.Unlock()
		batch.run()
	}
}
