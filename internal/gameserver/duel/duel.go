package duel

import (
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Player is a duellist as a duel reads and drives it. Its standing is the
// player's own: the duel sets it, and the player's damage path may set it
// to Interrupted at any time.
type Player interface {
	Standing
	ObjectID() int32
	CharacterName() string
	// Departed reports whether the player has begun leaving the world.
	Departed() bool
	SetDuelState(State)
	// JoinDuel puts the player in duel id, counting down.
	JoinDuel(id int32)
	Position() (x, y, z int)
	// PvPFlagged reports whether the player carries a PvP flag.
	PvPFlagged() bool
	// InDuelBlockedZone reports whether the player stands in a peace,
	// siege or PvP zone.
	InDuelBlockedZone() bool
}

// Manager owns every duel. mu guards all of its state, including every duel
// reachable from it.
type Manager[P Player] struct {
	mu     sync.Mutex
	nextID int32
	duels  map[int32]*fight[P]
}

type fight[P Player] struct {
	id    int32
	party bool
	// a challenged b; teamA and teamB are their sides, a and b alone in a
	// one-on-one duel, their parties in a party duel. A side is fixed when
	// the duel begins: a party edit cancels a party duel.
	a, b         P
	teamA, teamB []P
	endsAt       time.Time
	countdown    int
	// surrender is 1 or 2 once team 1 or team 2 surrendered.
	surrender int
	// over is set once the duel ended; the duel stays registered until
	// each of its players has left it (Leave), so its sides still resolve
	// for the updates its end causes.
	over bool
	// staying counts the players of an ended duel that have not left it.
	staying int
}

// View is a copy of one duel.
type View[P Player] struct {
	ID           int32
	Party        bool
	A, B         P
	TeamA, TeamB []P
}

// All returns the duel's players, team 1 first.
func (v View[P]) All() []P {
	return append(append([]P(nil), v.TeamA...), v.TeamB...)
}

// NewManager returns a manager with no duel.
func NewManager[P Player]() *Manager[P] {
	return &Manager[P]{duels: make(map[int32]*fight[P])}
}

// Begin starts, at now, a duel between a, the challenger, and b. In a party
// duel teamA and teamB are their parties; otherwise each side is its
// player alone and the teams are ignored. Every player of both sides joins
// the duel, counting down. A player already in a duel, which another duel
// accepted at the same time can have put it in, begins nothing.
func (m *Manager[P]) Begin(a, b P, teamA, teamB []P, party bool, now time.Time) (View[P], bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := &fight[P]{party: party, a: a, b: b, endsAt: now.Add(Length), countdown: soloCountdown}
	if party {
		f.countdown = partyCountdown
		f.teamA, f.teamB = slices.Clone(teamA), slices.Clone(teamB)
	} else {
		f.teamA, f.teamB = []P{a}, []P{b}
	}
	players := f.players()
	for _, p := range players {
		if p.DuelID() != 0 {
			return View[P]{}, false
		}
	}
	m.nextID++
	f.id = m.nextID
	for _, p := range players {
		p.JoinDuel(f.id)
	}
	m.duels[f.id] = f
	return f.view(), true
}

// StepKind is what one second of a duel does.
type StepKind uint8

// Steps.
const (
	// StepNone: the duel goes on with nothing to show.
	StepNone StepKind = iota
	// StepGone: the duel no longer runs; its ticks stop.
	StepGone
	// StepTeleport: both parties move to the Arena.
	StepTeleport
	// StepCountdown: both sides are told Seconds remain.
	StepCountdown
	// StepStart: the fight begins.
	StepStart
	// StepEnd: the duel ended with Result.
	StepEnd
)

// Step is one second of a duel.
type Step[P Player] struct {
	Kind    StepKind
	Seconds int
	Result  Result
	Duel    View[P]
}

// Tick runs the second of duel id that ends at now: it ends the duel when
// something ended it, counting down or fighting alike, and otherwise
// counts down. An ended duel reports StepEnd once, then StepGone.
func (m *Manager[P]) Tick(id int32, now time.Time) Step[P] {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.duels[id]
	if f == nil || f.over {
		return Step[P]{Kind: StepGone}
	}
	if res := f.check(now); res != Continue {
		m.end(f)
		return Step[P]{Kind: StepEnd, Result: res, Duel: f.view()}
	}
	if f.countdown < 0 {
		return Step[P]{Kind: StepNone, Duel: f.view()}
	}
	step := Step[P]{Kind: StepNone, Duel: f.view()}
	switch f.countdown {
	case teleportCountdown:
		if f.party {
			step.Kind = StepTeleport
		}
	case 30, 20, 15, 10, 3, 2, 1:
		step.Kind, step.Seconds = StepCountdown, f.countdown
	case 0:
		step.Kind = StepStart
	}
	f.countdown--
	return step
}

// check returns how the duel stands at now: Continue, or the result that
// ends it. A side whose leader left the world is defeated.
func (f *fight[P]) check(now time.Time) Result {
	aGone, bGone := f.a.Departed(), f.b.Departed()
	switch {
	case aGone && bGone:
		return Canceled
	case aGone:
		f.defeat(f.a)
		return Team1Surrender
	case bGone:
		f.defeat(f.b)
		return Team2Surrender
	case f.surrender == 1:
		return Team1Surrender
	case f.surrender == 2:
		return Team2Surrender
	case !now.Before(f.endsAt):
		return Timeout
	case f.a.DuelState() == Winner:
		return Team1Win
	case f.b.DuelState() == Winner:
		return Team2Win
	}
	for _, p := range f.teamA {
		if disturbed(p, f.b) {
			return Canceled
		}
	}
	for _, p := range f.teamB {
		if disturbed(p, f.a) {
			return Canceled
		}
	}
	return Continue
}

// disturbed reports whether p's state cancels its duel: it was
// interrupted, strayed out of Range of opponent, the other side's leader,
// carries a PvP flag, or stands in a peace, siege or PvP zone.
func disturbed[P Player](p, opponent P) bool {
	if p.DuelState() == Interrupted || p.PvPFlagged() || p.InDuelBlockedZone() {
		return true
	}
	px, py, pz := p.Position()
	ox, oy, oz := opponent.Position()
	return !location.In3DRadius(px, py, pz, ox, oy, oz, Range)
}

// Surrender gives up the duel p is in, for p's whole side: the side is
// defeated and the other one wins. A duel already surrendered, or p in no
// duel, changes nothing.
func (m *Manager[P]) Surrender(p P) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.live(p)
	if f == nil || f.surrender != 0 {
		return
	}
	var losers, winners []P
	switch {
	case f.party && contains(f.teamA, p), !f.party && p.ObjectID() == f.a.ObjectID():
		f.surrender, losers, winners = 1, f.teamA, f.teamB
	case f.party && contains(f.teamB, p), !f.party && p.ObjectID() == f.b.ObjectID():
		f.surrender, losers, winners = 2, f.teamB, f.teamA
	default:
		return
	}
	for _, l := range losers {
		l.SetDuelState(Dead)
	}
	for _, w := range winners {
		w.SetDuelState(Winner)
	}
}

// Defeat records p's defeat in its duel: p is Dead, and the other side
// wins once no player of p's side still fights.
func (m *Manager[P]) Defeat(p P) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f := m.live(p); f != nil {
		f.defeat(p)
	}
}

func (f *fight[P]) defeat(p P) {
	p.SetDuelState(Dead)
	if !f.party {
		if p.ObjectID() == f.a.ObjectID() {
			f.b.SetDuelState(Winner)
		} else {
			f.a.SetDuelState(Winner)
		}
		return
	}
	own, other := f.teamB, f.teamA
	if contains(f.teamA, p) {
		own, other = f.teamA, f.teamB
	}
	for _, m := range own {
		if m.DuelState() == Duelling {
			return
		}
	}
	for _, w := range other {
		w.SetDuelState(Winner)
	}
}

// PartyEdit cancels the party duel p is in, as any change to the members
// of a duelling party does, and returns it. A one-on-one duel, or p in no
// duel, is left alone.
func (m *Manager[P]) PartyEdit(p P) (View[P], bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.live(p)
	if f == nil || !f.party {
		return View[P]{}, false
	}
	m.end(f)
	return f.view(), true
}

// OppositeTeam returns the side fighting p in its duel: the other player
// of a one-on-one duel, the other party of a party duel. It still answers
// while an ended duel's players leave it.
func (m *Manager[P]) OppositeTeam(p P) []P {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.duels[p.DuelID()]
	if f == nil {
		return nil
	}
	switch {
	case contains(f.teamA, p):
		return slices.Clone(f.teamB)
	case contains(f.teamB, p):
		return slices.Clone(f.teamA)
	}
	return nil
}

// PartyDuel reports whether p's duel is a party duel. It still answers
// while an ended duel's players leave it; p in no duel is in none.
func (m *Manager[P]) PartyDuel(p P) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.duels[p.DuelID()]
	return f != nil && f.party
}

// Leave takes p out of the ended duel id once p's part of its end is
// applied; the duel is dropped once each of its players left it.
func (m *Manager[P]) Leave(id int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.duels[id]
	if f == nil || !f.over {
		return
	}
	if f.staying--; f.staying <= 0 {
		delete(m.duels, id)
	}
}

// end marks f over; its players then leave it one by one.
func (m *Manager[P]) end(f *fight[P]) {
	f.over = true
	f.staying = len(f.teamA) + len(f.teamB)
}

// live returns the running duel p is in.
func (m *Manager[P]) live(p P) *fight[P] {
	f := m.duels[p.DuelID()]
	if f == nil || f.over {
		return nil
	}
	return f
}

func (f *fight[P]) players() []P {
	return append(append([]P(nil), f.teamA...), f.teamB...)
}

func (f *fight[P]) view() View[P] {
	return View[P]{ID: f.id, Party: f.party, A: f.a, B: f.b, TeamA: slices.Clone(f.teamA), TeamB: slices.Clone(f.teamB)}
}

func contains[P Player](team []P, p P) bool {
	return slices.ContainsFunc(team, func(m P) bool { return m.ObjectID() == p.ObjectID() })
}
