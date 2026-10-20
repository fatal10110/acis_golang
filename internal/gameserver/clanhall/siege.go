package clanhall

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The siegable halls the siege runtime treats apart.
const (
	// FortressOfResistance starts its siege even with no attacker.
	FortressOfResistance int32 = 21
	// RainbowSprings keeps its attackers in its own list (see
	// RainbowDecrees), not in the stored siege attackers.
	RainbowSprings int32 = 62
)

// The hall siege system messages the siege package does not name. Each
// names the hall as a castle name parameter.
const (
	MsgSiegeFinished = 843 // hall name
	MsgClanDefeated  = 855 // clan name, hall name
	// msgNotRegistrationTime refuses a registration while the siege is
	// fought.
	msgNotRegistrationTime = 660
	// msgOtherSiegeRequested refuses a clan registered on another hall's
	// siege.
	msgOtherSiegeRequested = 639
)

// SiegeConfig is the siege.properties siegable hall settings.
type SiegeConfig struct {
	// MinClanLevel is the clan level a clan needs to register
	// (ChSiegeClanMinLevel).
	MinClanLevel int
	// MaxAttackers caps the attacking clans (ChAttackerMaxClans).
	MaxAttackers int
}

// DefaultSiegeConfig is the siegable hall configuration with every key at
// its default.
func DefaultSiegeConfig() SiegeConfig {
	return SiegeConfig{MinClanLevel: 4, MaxAttackers: 10}
}

// SiegeScript is the script running a siegable hall's siege. Its hooks run
// with no lock of the halls held and may read the halls back; they must not
// take a siege step themselves (Instant, End): an override runs the base
// steps through its SiegeBase instead.
type SiegeScript struct {
	// Winner names the clan that won the siege, 0 for none. The siege's
	// end asks it first.
	Winner func() int32
	// SpawnNPCs puts the hall's siege NPCs in the world as the siege
	// starts; UnspawnNPCs takes them out as it ends.
	SpawnNPCs, UnspawnNPCs func()
	// Prepare, Start and End, when set, run in place of the base steps:
	// the registrations' close (after delay, the siege's start is due),
	// the start and the end. The timers, Instant and End run them, one
	// step at a time; base runs the base steps as their part.
	Prepare func(base SiegeBase, delay time.Duration)
	Start   func(base SiegeBase)
	End     func(base SiegeBase)
	// OwnAttackers keeps the stored attackers out as the script binds the
	// siege: the script loads its attackers itself.
	OwnAttackers bool
	// DoorsUnattackable keeps the hall's doors out of everyone's attacks,
	// even while the siege is fought.
	DoorsUnattackable bool
}

// AttackerRow is one clanhall_siege_attackers row.
type AttackerRow struct {
	HallID, ClanID int32
}

// DecreeRow is one rainbowsprings_attacker_list row: the war decrees a clan
// handed in to fight for Rainbow Springs.
type DecreeRow struct {
	ClanID int32
	Count  int
}

// SiegeStore persists the hall sieges' attackers and the Rainbow Springs
// decrees.
type SiegeStore interface {
	LoadSiegeAttackers(ctx context.Context) ([]AttackerRow, error)
	// SaveSiegeAttackers replaces hall hallID's attackers with clanIDs.
	SaveSiegeAttackers(ctx context.Context, hallID int32, clanIDs []int32) error
	LoadRainbowDecrees(ctx context.Context) ([]DecreeRow, error)
	// AddRainbowDecree stores a clan's decrees.
	AddRainbowDecree(ctx context.Context, row DecreeRow) error
	DeleteRainbowDecree(ctx context.Context, clanID int32) error
	DeleteRainbowDecrees(ctx context.Context) error
}

// SiegeNotifier tells the world what the hall sieges do. Its calls are made
// with no lock of the halls held, so they may read the halls back.
type SiegeNotifier interface {
	// Announce sends m to every player in the world.
	Announce(m siege.Message)
	// SetSiegeState gives each member in the world of the clans the siege
	// state s, then resends its UserInfo and its relations.
	SetSiegeState(clans []*clan.Clan, s siege.State)
	// GuardDoors makes s the siege of each door named in gates.
	GuardDoors(gates []string, s door.Siege)
	// RespawnDoors stands each door named in gates again: a broken one
	// revives, each closes, and its HP is set to its maximum, or to half of
	// it when weak.
	RespawnDoors(gates []string, weak bool)
	// FlagPvP puts every player standing in field on the pvp flag, as
	// fighting outside a combat zone does, but for those standing in hall
	// hallID's grounds of another clan than ownerID: the step banished
	// them just before.
	FlagPvP(field *zone.Siege, hallID, ownerID int32)
}

// hallSiege is a siegable hall's siege state. Every field but field and
// restartPoints is changed holding both the halls' mu and their siegeMu,
// and read holding either; field is guarded by their fieldMu,
// restartPoints by their siegeMu.
type hallSiege struct {
	status siege.Status
	// next is the next siege's date, in Unix milliseconds.
	next int64
	// attackers are the attacking clans, in the order they joined.
	attackers []int32
	// script runs the siege; nil while no script runs it, when the hall
	// has no siege.
	script *SiegeScript
	// accomplished is set by the script once a clan won; previouslyOwned
	// tells the hall had an owner as its registrations closed.
	accomplished, previouslyOwned bool
	// task is the latest step armed; an earlier one still runs when due.
	task  *sim.Timer
	armed bool
	field *zone.Siege
	// restartPoints are where the players of a clan restart to town while
	// the siege's battlefield is on, by clan.
	restartPoints map[int32]location.Location
}

// HallSiege is one siegable hall's siege, as readers and its script see it.
type HallSiege struct {
	hs *Halls
	h  *hall
}

// A hall siege is the door.Siege of its hall's gates.
var _ door.Siege = (*HallSiege)(nil)

// Siege returns siegable hall hallID's siege.
func (hs *Halls) Siege(hallID int32) (*HallSiege, bool) {
	if hs == nil {
		return nil, false
	}
	h, ok := hs.byID[hallID]
	if !ok || h.siege == nil {
		return nil, false
	}
	return &HallSiege{hs: hs, h: h}, true
}

// sieges lists the siegable halls, by id.
func (hs *Halls) sieges() []*hall {
	var out []*hall
	for _, h := range hs.order {
		if h.siege != nil {
			out = append(out, h)
		}
	}
	return out
}

// SiegeInProgress reports whether siegable hall hallID's siege is being
// fought.
func (hs *Halls) SiegeInProgress(hallID int32) bool {
	s, ok := hs.Siege(hallID)
	return ok && s.InProgress()
}

// ActiveSiegeAt returns the hall siege whose battlefield is on and holds the
// point (x, y, z). A hall with no script running its siege has none.
func (hs *Halls) ActiveSiegeAt(x, y, z int) (*HallSiege, bool) {
	if hs == nil {
		return nil, false
	}
	for _, h := range hs.sieges() {
		s := &HallSiege{hs: hs, h: h}
		if f := s.Battlefield(); f != nil && f.Active() && f.ContainsPoint(x, y, z) && s.Scripted() {
			return s, true
		}
	}
	return nil, false
}

// ClanParticipating reports whether clanID attacks a hall's siege.
func (hs *Halls) ClanParticipating(clanID int32) bool {
	if hs == nil || clanID == 0 {
		return false
	}
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	return hs.participatingLocked(clanID)
}

// Besieging reports whether clanID attacks a hall siege being fought.
func (hs *Halls) Besieging(clanID int32) bool {
	if hs == nil || clanID == 0 {
		return false
	}
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	for _, h := range hs.order {
		if s := h.siege; s != nil && s.status == siege.StatusInProgress && slices.Contains(s.attackers, clanID) {
			return true
		}
	}
	return false
}

func (hs *Halls) participatingLocked(clanID int32) bool {
	for _, h := range hs.order {
		if s := h.siege; s != nil && s.script != nil && slices.Contains(s.attackers, clanID) {
			return true
		}
	}
	return false
}

// ID is the hall's id.
func (s *HallSiege) ID() int32 { return s.h.id() }

// Gates are the hall's gates (clanHalls.xml <gates>).
func (s *HallSiege) Gates() []string { return s.h.data.Gates }

// Status is where the siege stands in its cycle.
func (s *HallSiege) Status() siege.Status {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return s.h.siege.status
}

// InProgress reports whether the siege is being fought.
func (s *HallSiege) InProgress() bool { return s.Status() == siege.StatusInProgress }

// WaitingBattle reports whether the registrations closed and the siege has
// not started.
func (s *HallSiege) WaitingBattle() bool { return s.Status() == siege.StatusRegistrationOver }

// Date is the next siege's date, in Unix milliseconds.
func (s *HallSiege) Date() int64 {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return s.h.siege.next
}

// Scripted reports whether a script runs the siege: a hall without one has
// no siege, no attacker and no step.
func (s *HallSiege) Scripted() bool {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return s.h.siege.script != nil
}

// Attackers are the attacking clans, in the order they joined.
func (s *HallSiege) Attackers() []int32 {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return slices.Clone(s.h.siege.attackers)
}

// Registered reports whether clanID attacks the siege.
func (s *HallSiege) Registered(clanID int32) bool {
	if clanID == 0 {
		return false
	}
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return slices.Contains(s.h.siege.attackers, clanID)
}

// Attacking reports whether the siege is being fought and clanID attacks
// it.
func (s *HallSiege) Attacking(clanID int32) bool {
	if clanID == 0 {
		return false
	}
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	hs := s.h.siege
	return hs.status == siege.StatusInProgress && slices.Contains(hs.attackers, clanID)
}

// SetMissionAccomplished records whether a clan won the siege; the siege's
// end then gives the hall to the script's winner.
func (s *HallSiege) SetMissionAccomplished(v bool) {
	s.hs.mu.Lock()
	defer s.hs.mu.Unlock()
	s.hs.setSiege(func() { s.h.siege.accomplished = v })
}

// PreviouslyOwned reports whether the hall had an owner as its
// registrations closed.
func (s *HallSiege) PreviouslyOwned() bool {
	s.hs.siegeMu.Lock()
	defer s.hs.siegeMu.Unlock()
	return s.h.siege.previouslyOwned
}

// Battlefield is the siege zone the siege turns on, nil for none.
func (s *HallSiege) Battlefield() *zone.Siege {
	s.hs.fieldMu.Lock()
	defer s.hs.fieldMu.Unlock()
	return s.h.siege.field
}

// withBattlefield runs fn on the siege's battlefield, when it has one, with
// the battlefields held.
func (s *HallSiege) withBattlefield(fn func(f *zone.Siege)) {
	s.hs.fieldMu.Lock()
	defer s.hs.fieldMu.Unlock()
	if f := s.h.siege.field; f != nil {
		fn(f)
	}
}

// AllowsDoorAttack reports whether attacker may attack one of the hall's
// doors: while the siege is fought, a player of an attacking clan or its
// summon, unless the script keeps the doors out of attacks.
func (s *HallSiege) AllowsDoorAttack(attacker skilltarget.Actor, _ bool) bool {
	if sc := s.script(); sc != nil && sc.DoorsUnattackable {
		return false
	}
	return s.Attacking(siege.ActingClanID(attacker))
}

// DoorHit is what the hall's siege makes of a hit on one of its doors: it
// lowers the door's HP while the battlefield is on, and is harmless
// otherwise.
func (s *HallSiege) DoorHit(attackable.Combatant) door.HitOutcome {
	if f := s.Battlefield(); f != nil && f.Active() {
		return door.HitDamages
	}
	return door.HitHarmless
}

// ShowsDoorHP reports false: a hall's doors never show their HP.
func (s *HallSiege) ShowsDoorHP() bool { return false }

// DoorBroken tells nobody: a hall's broken door goes unannounced.
func (s *HallSiege) DoorBroken(bool) {}

// RegisterResult is how a registration ended.
type RegisterResult uint8

const (
	// Registered made the clan an attacker.
	Registered RegisterResult = iota
	// RegisterRefused refused it; the message tells why.
	RegisterRefused
	// RegisterNoSiege found no script running the siege: the hall takes no
	// attacker.
	RegisterNoSiege
)

// Register makes cl an attacker of the siege, refusing, in this order, a
// clan below the configured level, a siege whose registrations closed or
// that is fought, the hall's owner, a clan owning a castle or a hall, a
// clan registered already, here or on another hall's siege, and a full
// attacker side. Registering is stored.
func (s *HallSiege) Register(cl *clan.Clan) (RegisterResult, siege.Message) {
	hs := s.hs
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	id := s.h.id()
	st := s.h.siege
	refuse := func(m siege.Message) (RegisterResult, siege.Message) { return RegisterRefused, m }
	switch {
	case cl == nil || cl.Level() < hs.siegeCfg.MinClanLevel:
		return refuse(siege.Message{ID: siege.MsgOnlyClanLevel4})
	case st.status == siege.StatusRegistrationOver:
		return refuse(siege.Message{ID: siege.MsgDeadlinePassed, CastleID: int(id)})
	case st.status == siege.StatusInProgress:
		return refuse(siege.Message{ID: msgNotRegistrationTime})
	case s.h.ownerID == cl.ID():
		return refuse(siege.Message{ID: siege.MsgOwnerAutomaticallyDefends})
	case cl.CastleID() != 0 || cl.HallID() != 0:
		return refuse(siege.Message{ID: siege.MsgOwnerCannotJoinOther})
	case st.script == nil:
		return RegisterNoSiege, siege.Message{}
	case slices.Contains(st.attackers, cl.ID()):
		return refuse(siege.Message{ID: siege.MsgAlreadyRequested})
	case hs.participatingLocked(cl.ID()):
		return refuse(siege.Message{ID: msgOtherSiegeRequested})
	case len(st.attackers) >= hs.siegeCfg.MaxAttackers:
		return refuse(siege.Message{ID: siege.MsgAttackerSideFull})
	}
	st.attackers = append(st.attackers, cl.ID())
	hs.saveAttackersLocked(s.h)
	return Registered, siege.Message{}
}

// Unregister drops clanID from the attackers, and stores it, while the
// siege takes registrations.
func (s *HallSiege) Unregister(clanID int32) {
	hs := s.hs
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if s.h.siege.status != siege.StatusRegistrationOpened {
		return
	}
	hs.setSiege(func() { hs.removeAttackerLocked(s.h, clanID) })
}

// removeAttackerLocked drops clanID from h's attackers and stores it. The
// halls' mu and siegeMu are held.
func (hs *Halls) removeAttackerLocked(h *hall, clanID int32) {
	st := h.siege
	if st.script == nil {
		return
	}
	i := slices.Index(st.attackers, clanID)
	if i < 0 {
		return
	}
	st.attackers = slices.Delete(st.attackers, i, i+1)
	hs.saveAttackersLocked(h)
}

// ForgetClan drops cl, a clan being destroyed, from the hall sieges: a clan
// owning no hall leaves every siege's attackers; a clan owning a siegable
// hall loses it.
func (hs *Halls) ForgetClan(cl *clan.Clan) {
	if hs == nil {
		return
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hallID := cl.HallID()
	if hallID == 0 {
		hs.siegeMu.Lock()
		for _, h := range hs.sieges() {
			hs.removeAttackerLocked(h, cl.ID())
		}
		hs.siegeMu.Unlock()
		return
	}
	if h, ok := hs.byID[hallID]; ok && h.siege != nil && h.ownerID == cl.ID() {
		hs.freeLocked(h)
	}
}

// saveAttackersLocked stores h's attackers, on h's lane. Rainbow Springs
// keeps none.
func (hs *Halls) saveAttackersLocked(h *hall) {
	if h.id() == RainbowSprings {
		return
	}
	id, clans := h.id(), slices.Clone(h.siege.attackers)
	hs.writeSiege("store hall siege attackers", id, func(ctx context.Context, st SiegeStore) error {
		return st.SaveSiegeAttackers(ctx, id, clans)
	})
}

// writeSiege queues fn on hallID's persistence lane, behind the hall's row
// writes, or runs it at once without a writer. Without a siege store it
// writes nothing.
func (hs *Halls) writeSiege(what string, hallID int32, fn func(context.Context, SiegeStore) error) {
	store, log := hs.siegeStore, hs.log
	if store == nil {
		return
	}
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("hall_id", hallID).Msg("clanhall: " + what)
		}
	}
	if hs.writes == nil {
		job()
		return
	}
	if !hs.writes.Enqueue(hallID, job) {
		log.Error().Int32("hall_id", hallID).Msg("clanhall: " + what + ": write dropped")
	}
}

// RestoreSieges loads the stored siege attackers and the Rainbow Springs
// decrees from store, which then stores their changes. Run it once at boot,
// after Restore. A hall takes its stored attackers as a script binds its
// siege.
func (hs *Halls) RestoreSieges(ctx context.Context, store SiegeStore) error {
	if hs == nil || store == nil {
		return nil
	}
	rows, err := store.LoadSiegeAttackers(ctx)
	if err != nil {
		return fmt.Errorf("load hall siege attackers: %w", err)
	}
	decrees, err := store.LoadRainbowDecrees(ctx)
	if err != nil {
		return fmt.Errorf("load rainbow springs decrees: %w", err)
	}
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	hs.siegeStore = store
	hs.storedAttackers = map[int32][]int32{}
	for _, r := range rows {
		hs.storedAttackers[r.HallID] = append(hs.storedAttackers[r.HallID], r.ClanID)
	}
	hs.decrees = map[int32]int{}
	for _, d := range decrees {
		hs.decrees[d.ClanID] = d.Count
	}
	return nil
}

// RainbowDecrees returns the decrees each clan handed in for Rainbow
// Springs.
func (hs *Halls) RainbowDecrees() map[int32]int {
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	out := make(map[int32]int, len(hs.decrees))
	for k, v := range hs.decrees {
		out[k] = v
	}
	return out
}

// AddRainbowDecrees records and stores the decrees clanID handed in.
func (hs *Halls) AddRainbowDecrees(clanID int32, count int) {
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	if hs.decrees == nil {
		hs.decrees = map[int32]int{}
	}
	hs.decrees[clanID] = count
	row := DecreeRow{ClanID: clanID, Count: count}
	hs.writeSiege("store rainbow springs decree", RainbowSprings, func(ctx context.Context, st SiegeStore) error {
		return st.AddRainbowDecree(ctx, row)
	})
}

// RemoveRainbowDecrees drops, and stores, the decrees clanID handed in.
func (hs *Halls) RemoveRainbowDecrees(clanID int32) {
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	delete(hs.decrees, clanID)
	hs.writeSiege("delete rainbow springs decree", RainbowSprings, func(ctx context.Context, st SiegeStore) error {
		return st.DeleteRainbowDecree(ctx, clanID)
	})
}

// ClearRainbowDecrees drops, and stores, every clan's decrees.
func (hs *Halls) ClearRainbowDecrees() {
	hs.siegeMu.Lock()
	defer hs.siegeMu.Unlock()
	clear(hs.decrees)
	hs.writeSiege("delete rainbow springs decrees", RainbowSprings, func(ctx context.Context, st SiegeStore) error {
		return st.DeleteRainbowDecrees(ctx)
	})
}

// BindSiege makes script run siegable hall hallID's siege, reporting false
// for a hall that is not siegable, has no stored row and so no siege date,
// or whose siege a script runs already. The
// hall takes its stored attackers that are still clans, Rainbow Springs and
// a script loading its own attackers none, and its registrations close an hour before its siege date: at once
// when that hour has come. Once the sieges run (RunSieges) that step is
// armed now, before then as they start.
func (hs *Halls) BindSiege(hallID int32, script SiegeScript) (*HallSiege, bool) {
	if hs == nil {
		return nil, false
	}
	h, ok := hs.byID[hallID]
	if !ok || h.siege == nil {
		return nil, false
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	st := h.siege
	if st.script != nil || st.next == 0 {
		return nil, false
	}
	var stored []int32
	if hallID != RainbowSprings && !script.OwnAttackers {
		hs.siegeMu.Lock()
		rows := hs.storedAttackers[hallID]
		hs.siegeMu.Unlock()
		for _, id := range rows {
			if _, ok := hs.clans.Get(id); ok && !slices.Contains(stored, id) {
				stored = append(stored, id)
			}
		}
	}
	sc := script
	hs.setSiege(func() {
		st.script = &sc
		st.attackers = append(st.attackers, stored...)
	})
	if hs.siegeNotify != nil {
		hs.armPrepareLocked(h)
	}
	return &HallSiege{hs: hs, h: h}, true
}

// RunSieges runs the hall sieges with cfg from now on, telling the world
// through notify, each turning on the siege zone of fields whose residence
// is its hall. Every hall's siege guards its gates; each siege a script
// runs already has its registrations' close armed. Call it once, after
// Start.
func (hs *Halls) RunSieges(cfg SiegeConfig, notify SiegeNotifier, fields []*zone.Siege) {
	if hs == nil {
		return
	}
	hs.RebindBattlefields(fields)
	if notify != nil {
		for _, h := range hs.sieges() {
			if gates := h.data.Gates; len(gates) > 0 {
				notify.GuardDoors(gates, &HallSiege{hs: hs, h: h})
			}
		}
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.siegeCfg, hs.siegeNotify = cfg, notify
	for _, h := range hs.sieges() {
		if h.siege.script != nil && !h.siege.armed {
			hs.armPrepareLocked(h)
		}
	}
}

// RebindBattlefields moves each hall siege onto the battlefield of fields
// whose residence is its hall (none when fields has none), as a zone reload
// reads the battlefields anew: the new battlefield is on while the one it
// replaces is. Call it before fields hold anyone.
func (hs *Halls) RebindBattlefields(fields []*zone.Siege) {
	if hs == nil {
		return
	}
	hs.fieldMu.Lock()
	defer hs.fieldMu.Unlock()
	for _, h := range hs.sieges() {
		var next *zone.Siege
		for _, f := range fields {
			if int32(f.ResidenceID) == h.id() {
				next = f
				break
			}
		}
		if next != nil && h.siege.field != nil && h.siege.field.Active() {
			next.SetActive(true)
		}
		h.siege.field = next
	}
}

// siegeDelay is how long from now until ms, in Unix milliseconds, on the
// halls' clock.
func (hs *Halls) siegeDelay(ms int64) time.Duration {
	return time.Duration(ms-hs.nowLocked()) * time.Millisecond
}
