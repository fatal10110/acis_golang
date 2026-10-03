package duel

import (
	"slices"
	"testing"
	"time"
)

// fakePlayer is a duellist whose departure, position, flag and zone the
// test sets directly. The manager's lock serializes every access.
type fakePlayer struct {
	id       int32
	duelID   int32
	state    State
	departed bool
	flagged  bool
	blocked  bool
	x, y, z  int
}

func (p *fakePlayer) DuelID() int32           { return p.duelID }
func (p *fakePlayer) DuelState() State        { return p.state }
func (p *fakePlayer) ObjectID() int32         { return p.id }
func (p *fakePlayer) CharacterName() string   { return "p" }
func (p *fakePlayer) Departed() bool          { return p.departed }
func (p *fakePlayer) SetDuelState(s State)    { p.state = s }
func (p *fakePlayer) JoinDuel(id int32)       { p.duelID, p.state = id, Countdown }
func (p *fakePlayer) Position() (x, y, z int) { return p.x, p.y, p.z }
func (p *fakePlayer) PvPFlagged() bool        { return p.flagged }
func (p *fakePlayer) InDuelBlockedZone() bool { return p.blocked }
func (p *fakePlayer) fight()                  { p.state = Duelling }
func newFake(id int32, x int) *fakePlayer     { return &fakePlayer{id: id, x: x} }
func fakes(ps ...*fakePlayer) []*fakePlayer   { return ps }

func allFight(ps ...*fakePlayer) {
	for _, p := range ps {
		p.fight()
	}
}

var t0 = time.Unix(1_000_000, 0)

// solo begins a one-on-one duel between a challenger at x=0 and a
// challenged player at x=100, both fighting.
func solo(t *testing.T) (*Manager[*fakePlayer], *fakePlayer, *fakePlayer) {
	t.Helper()
	m := NewManager[*fakePlayer]()
	a, b := newFake(1, 0), newFake(2, 100)
	if _, ok := m.Begin(a, b, nil, nil, false, t0); !ok {
		t.Fatal("Begin refused a free pair")
	}
	allFight(a, b)
	return m, a, b
}

// partyDuel begins a party duel: a and a2 against b and b2, all fighting.
func partyDuel(t *testing.T) (m *Manager[*fakePlayer], a, a2, b, b2 *fakePlayer) {
	t.Helper()
	m = NewManager[*fakePlayer]()
	a, a2, b, b2 = newFake(1, 0), newFake(3, 50), newFake(2, 100), newFake(4, 150)
	if _, ok := m.Begin(a, b, fakes(a, a2), fakes(b, b2), true, t0); !ok {
		t.Fatal("Begin refused free parties")
	}
	allFight(a, a2, b, b2)
	return m, a, a2, b, b2
}

func tickResult(t *testing.T, m *Manager[*fakePlayer], id int32, now time.Time) Result {
	t.Helper()
	step := m.Tick(id, now)
	switch step.Kind {
	case StepEnd:
		return step.Result
	case StepGone:
		t.Fatal("Tick reported a gone duel")
	}
	return Continue
}

// TestCheckOrderSolo walks each end condition of a one-on-one duel in the
// reference order: departures, surrender, timeout, a winner, then the
// cancelling disturbances.
func TestCheckOrderSolo(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Manager[*fakePlayer], a, b *fakePlayer)
		after time.Duration
		want  Result
		wantA State
		wantB State
	}{
		{name: "nothing", setup: func(_ *Manager[*fakePlayer], _, _ *fakePlayer) {}, want: Continue, wantA: Duelling, wantB: Duelling},
		{name: "both departed", setup: func(_ *Manager[*fakePlayer], a, b *fakePlayer) { a.departed, b.departed = true, true }, want: Canceled, wantA: Duelling, wantB: Duelling},
		{name: "A departed", setup: func(_ *Manager[*fakePlayer], a, _ *fakePlayer) { a.departed = true }, want: Team1Surrender, wantA: Dead, wantB: Winner},
		{name: "B departed", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.departed = true }, want: Team2Surrender, wantA: Winner, wantB: Dead},
		{name: "departure beats surrender", setup: func(m *Manager[*fakePlayer], a, b *fakePlayer) { m.Surrender(b); a.departed = true }, want: Team1Surrender, wantA: Dead, wantB: Winner},
		{name: "A surrendered", setup: func(m *Manager[*fakePlayer], a, _ *fakePlayer) { m.Surrender(a) }, want: Team1Surrender, wantA: Dead, wantB: Winner},
		{name: "B surrendered", setup: func(m *Manager[*fakePlayer], _, b *fakePlayer) { m.Surrender(b) }, want: Team2Surrender, wantA: Winner, wantB: Dead},
		{name: "surrender beats timeout", setup: func(m *Manager[*fakePlayer], a, _ *fakePlayer) { m.Surrender(a) }, after: Length, want: Team1Surrender, wantA: Dead, wantB: Winner},
		{name: "timeout", setup: func(_ *Manager[*fakePlayer], _, _ *fakePlayer) {}, after: Length, want: Timeout, wantA: Duelling, wantB: Duelling},
		{name: "timeout beats winner", setup: func(_ *Manager[*fakePlayer], a, _ *fakePlayer) { a.state = Winner }, after: Length, want: Timeout, wantA: Winner, wantB: Duelling},
		{name: "A winner", setup: func(_ *Manager[*fakePlayer], a, _ *fakePlayer) { a.state = Winner }, want: Team1Win, wantA: Winner, wantB: Duelling},
		{name: "B winner", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.state = Winner }, want: Team2Win, wantA: Duelling, wantB: Winner},
		{name: "winner beats interrupt", setup: func(_ *Manager[*fakePlayer], a, b *fakePlayer) { a.state, b.state = Winner, Interrupted }, want: Team1Win, wantA: Winner, wantB: Interrupted},
		{name: "A interrupted", setup: func(_ *Manager[*fakePlayer], a, _ *fakePlayer) { a.state = Interrupted }, want: Canceled, wantA: Interrupted, wantB: Duelling},
		{name: "B interrupted", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.state = Interrupted }, want: Canceled, wantA: Duelling, wantB: Interrupted},
		{name: "just in range", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.x = Range - 1 }, want: Continue, wantA: Duelling, wantB: Duelling},
		{name: "out of range", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.x = Range }, want: Canceled, wantA: Duelling, wantB: Duelling},
		{name: "A flagged", setup: func(_ *Manager[*fakePlayer], a, _ *fakePlayer) { a.flagged = true }, want: Canceled, wantA: Duelling, wantB: Duelling},
		{name: "B flagged", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.flagged = true }, want: Canceled, wantA: Duelling, wantB: Duelling},
		{name: "A in blocked zone", setup: func(_ *Manager[*fakePlayer], a, _ *fakePlayer) { a.blocked = true }, want: Canceled, wantA: Duelling, wantB: Duelling},
		{name: "B in blocked zone", setup: func(_ *Manager[*fakePlayer], _, b *fakePlayer) { b.blocked = true }, want: Canceled, wantA: Duelling, wantB: Duelling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, a, b := solo(t)
			tc.setup(m, a, b)
			if got := tickResult(t, m, a.DuelID(), t0.Add(tc.after)); got != tc.want {
				t.Fatalf("result = %d, want %d", got, tc.want)
			}
			if a.state != tc.wantA || b.state != tc.wantB {
				t.Fatalf("states = (%d, %d), want (%d, %d)", a.state, b.state, tc.wantA, tc.wantB)
			}
		})
	}
}

// TestCheckOrderParty walks the end conditions of a party duel: only the
// leaders' departures end it, any member's surrender gives up its whole
// side, and each member's disturbance, measured against the other side's
// leader, cancels it.
func TestCheckOrderParty(t *testing.T) {
	type fn = func(m *Manager[*fakePlayer], a, a2, b, b2 *fakePlayer)
	cases := []struct {
		name  string
		setup fn
		after time.Duration
		want  Result
		// winners lists who must stand Winner afterwards, losers Dead.
		winners, losers func(a, a2, b, b2 *fakePlayer) []*fakePlayer
	}{
		{name: "nothing", setup: func(_ *Manager[*fakePlayer], _, _, _, _ *fakePlayer) {}, want: Continue},
		{name: "member departed", setup: func(_ *Manager[*fakePlayer], _, a2, _, b2 *fakePlayer) { a2.departed, b2.departed = true, true }, want: Continue},
		{name: "both leaders departed", setup: func(_ *Manager[*fakePlayer], a, _, b, _ *fakePlayer) { a.departed, b.departed = true, true }, want: Canceled},
		{
			name: "leader A departed", setup: func(_ *Manager[*fakePlayer], a, _, _, _ *fakePlayer) { a.departed = true }, want: Team1Surrender,
			losers: func(a, _, _, _ *fakePlayer) []*fakePlayer { return fakes(a) },
		},
		{
			name: "leader B departed", setup: func(_ *Manager[*fakePlayer], _, _, b, _ *fakePlayer) { b.departed = true }, want: Team2Surrender,
			losers: func(_, _, b, _ *fakePlayer) []*fakePlayer { return fakes(b) },
		},
		{
			name: "member of A surrendered", setup: func(m *Manager[*fakePlayer], _, a2, _, _ *fakePlayer) { m.Surrender(a2) }, want: Team1Surrender,
			losers:  func(a, a2, _, _ *fakePlayer) []*fakePlayer { return fakes(a, a2) },
			winners: func(_, _, b, b2 *fakePlayer) []*fakePlayer { return fakes(b, b2) },
		},
		{
			name: "member of B surrendered", setup: func(m *Manager[*fakePlayer], _, _, _, b2 *fakePlayer) { m.Surrender(b2) }, want: Team2Surrender,
			losers:  func(_, _, b, b2 *fakePlayer) []*fakePlayer { return fakes(b, b2) },
			winners: func(a, a2, _, _ *fakePlayer) []*fakePlayer { return fakes(a, a2) },
		},
		{name: "timeout", setup: func(_ *Manager[*fakePlayer], _, _, _, _ *fakePlayer) {}, after: Length, want: Timeout},
		{name: "leader A winner", setup: func(_ *Manager[*fakePlayer], a, _, _, _ *fakePlayer) { a.state = Winner }, want: Team1Win},
		{name: "leader B winner", setup: func(_ *Manager[*fakePlayer], _, _, b, _ *fakePlayer) { b.state = Winner }, want: Team2Win},
		{name: "member winner alone", setup: func(_ *Manager[*fakePlayer], _, a2, _, _ *fakePlayer) { a2.state = Winner }, want: Continue},
		{name: "member of A interrupted", setup: func(_ *Manager[*fakePlayer], _, a2, _, _ *fakePlayer) { a2.state = Interrupted }, want: Canceled},
		{name: "member of B interrupted", setup: func(_ *Manager[*fakePlayer], _, _, _, b2 *fakePlayer) { b2.state = Interrupted }, want: Canceled},
		// a2 stays near its own leader but strays from b, the other leader.
		{name: "member of A out of range", setup: func(_ *Manager[*fakePlayer], _, a2, _, _ *fakePlayer) { a2.x = 100 - Range }, want: Canceled},
		{name: "member of B out of range", setup: func(_ *Manager[*fakePlayer], _, _, _, b2 *fakePlayer) { b2.x = Range }, want: Canceled},
		{name: "member of A flagged", setup: func(_ *Manager[*fakePlayer], _, a2, _, _ *fakePlayer) { a2.flagged = true }, want: Canceled},
		{name: "member of B flagged", setup: func(_ *Manager[*fakePlayer], _, _, _, b2 *fakePlayer) { b2.flagged = true }, want: Canceled},
		{name: "member of A in blocked zone", setup: func(_ *Manager[*fakePlayer], _, a2, _, _ *fakePlayer) { a2.blocked = true }, want: Canceled},
		{name: "member of B in blocked zone", setup: func(_ *Manager[*fakePlayer], _, _, _, b2 *fakePlayer) { b2.blocked = true }, want: Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, a, a2, b, b2 := partyDuel(t)
			tc.setup(m, a, a2, b, b2)
			if got := tickResult(t, m, a.DuelID(), t0.Add(tc.after)); got != tc.want {
				t.Fatalf("result = %d, want %d", got, tc.want)
			}
			if tc.winners != nil {
				for _, p := range tc.winners(a, a2, b, b2) {
					if p.state != Winner {
						t.Fatalf("player %d state = %d, want Winner", p.id, p.state)
					}
				}
			}
			if tc.losers != nil {
				for _, p := range tc.losers(a, a2, b, b2) {
					if p.state != Dead {
						t.Fatalf("player %d state = %d, want Dead", p.id, p.state)
					}
				}
			}
		})
	}
}

// TestDefeatSolo: one defeat ends a one-on-one duel, the other player
// winning.
func TestDefeatSolo(t *testing.T) {
	m, a, b := solo(t)
	m.Defeat(a)
	if a.state != Dead || b.state != Winner {
		t.Fatalf("states = (%d, %d), want (Dead, Winner)", a.state, b.state)
	}
	if got := tickResult(t, m, a.DuelID(), t0); got != Team2Win {
		t.Fatalf("result = %d, want Team2Win", got)
	}
}

// TestDefeatPartyWaitsForWholeSide: the other party wins only once no
// member of the defeated side still fights.
func TestDefeatPartyWaitsForWholeSide(t *testing.T) {
	m, a, a2, b, b2 := partyDuel(t)
	m.Defeat(a)
	if a.state != Dead || a2.state != Duelling || b.state != Duelling || b2.state != Duelling {
		t.Fatalf("after first defeat states = %d %d %d %d", a.state, a2.state, b.state, b2.state)
	}
	if got := tickResult(t, m, a.DuelID(), t0); got != Continue {
		t.Fatalf("result after first defeat = %d, want Continue", got)
	}
	m.Defeat(a2)
	if b.state != Winner || b2.state != Winner {
		t.Fatalf("winners = %d %d, want Winner", b.state, b2.state)
	}
	if got := tickResult(t, m, a.DuelID(), t0); got != Team2Win {
		t.Fatalf("result = %d, want Team2Win", got)
	}
}

// TestSurrenderOnce: a second surrender, from either side, changes nothing.
func TestSurrenderOnce(t *testing.T) {
	m, a, b := solo(t)
	m.Surrender(a)
	m.Surrender(b)
	if a.state != Dead || b.state != Winner {
		t.Fatalf("states = (%d, %d), want (Dead, Winner)", a.state, b.state)
	}
	if got := tickResult(t, m, a.DuelID(), t0); got != Team1Surrender {
		t.Fatalf("result = %d, want Team1Surrender", got)
	}
}

// TestBeginRefusesPlayerInDuel: a player already in a duel cannot be taken
// into a second one, and nobody of the refused duel joins it.
func TestBeginRefusesPlayerInDuel(t *testing.T) {
	m, a, b := solo(t)
	c := newFake(5, 0)
	if _, ok := m.Begin(c, a, nil, nil, false, t0); ok {
		t.Fatal("Begin took a duellist into a second duel")
	}
	if c.duelID != 0 || c.state != NoDuel {
		t.Fatalf("refused challenger joined: id %d state %d", c.duelID, c.state)
	}
	d, e := newFake(6, 0), newFake(7, 0)
	if _, ok := m.Begin(d, e, fakes(d, b), fakes(e), true, t0); ok {
		t.Fatal("Begin took a party with a duellist into a party duel")
	}
	if d.duelID != 0 || e.duelID != 0 {
		t.Fatalf("refused party duel joined players: %d %d", d.duelID, e.duelID)
	}
	if a.duelID != b.duelID || a.duelID == 0 {
		t.Fatalf("the first duel lost its players: %d %d", a.duelID, b.duelID)
	}
}

// TestPartyEdit: a party edit cancels a party duel and leaves a one-on-one
// duel alone.
func TestPartyEdit(t *testing.T) {
	m, a, _ := solo(t)
	if _, ok := m.PartyEdit(a); ok {
		t.Fatal("PartyEdit cancelled a one-on-one duel")
	}
	if step := m.Tick(a.DuelID(), t0); step.Kind == StepEnd || step.Kind == StepGone {
		t.Fatalf("one-on-one duel stopped after PartyEdit: step %d", step.Kind)
	}

	pm, pa, _, _, _ := partyDuel(t)
	v, ok := pm.PartyEdit(pa)
	if !ok || v.ID != pa.DuelID() || !v.Party {
		t.Fatalf("PartyEdit = (%+v, %v), want the party duel", v, ok)
	}
	if step := pm.Tick(pa.DuelID(), t0); step.Kind != StepGone {
		t.Fatalf("cancelled party duel ticked step %d, want StepGone", step.Kind)
	}
	if _, ok := pm.PartyEdit(pa); ok {
		t.Fatal("a second PartyEdit cancelled the ended duel again")
	}
}

// TestEndReportsOnceThenGone: an ended duel reports StepEnd once.
func TestEndReportsOnceThenGone(t *testing.T) {
	m, a, _ := solo(t)
	m.Surrender(a)
	if step := m.Tick(a.DuelID(), t0); step.Kind != StepEnd {
		t.Fatalf("first tick = %d, want StepEnd", step.Kind)
	}
	if step := m.Tick(a.DuelID(), t0); step.Kind != StepGone {
		t.Fatalf("second tick = %d, want StepGone", step.Kind)
	}
}

// TestLeaveDropsAfterLastPlayer: an ended duel still resolves its sides
// until every one of its players left it.
func TestLeaveDropsAfterLastPlayer(t *testing.T) {
	m, a, a2, b, b2 := partyDuel(t)
	id := a.DuelID()
	m.Leave(id) // a running duel ignores Leave
	if got := m.OppositeTeam(a); len(got) != 2 {
		t.Fatalf("running duel opposite team = %d players, want 2", len(got))
	}
	m.Surrender(a)
	if step := m.Tick(id, t0); step.Kind != StepEnd {
		t.Fatalf("tick = %d, want StepEnd", step.Kind)
	}
	for i := range 3 {
		m.Leave(id)
		if got := m.OppositeTeam(b2); len(got) != 2 || got[0] != a || got[1] != a2 {
			t.Fatalf("after %d leaves opposite team = %v, want team A", i+1, got)
		}
	}
	m.Leave(id)
	if got := m.OppositeTeam(b); got != nil {
		t.Fatalf("after the last leave opposite team = %v, want none", got)
	}
}

// TestCountdown: a one-on-one duel counts 3, 2, 1 then starts; a party
// duel moves to the arena at 33 and counts from 30.
func TestCountdown(t *testing.T) {
	m, a, _ := solo(t)
	var got []int
	for range soloCountdown + 2 {
		s := m.Tick(a.DuelID(), t0)
		switch s.Kind {
		case StepCountdown:
			got = append(got, s.Seconds)
		case StepStart:
			got = append(got, 0)
		case StepTeleport:
			t.Fatal("one-on-one duel teleported")
		}
	}
	if want := []int{3, 2, 1, 0}; !slices.Equal(got, want) {
		t.Fatalf("solo countdown = %v, want %v", got, want)
	}

	pm, pa, _, _, _ := partyDuel(t)
	got = nil
	teleportAt := -1
	for i := range partyCountdown + 2 {
		s := pm.Tick(pa.DuelID(), t0)
		switch s.Kind {
		case StepCountdown:
			got = append(got, s.Seconds)
		case StepStart:
			got = append(got, 0)
		case StepTeleport:
			teleportAt = partyCountdown - i
		}
	}
	if teleportAt != teleportCountdown {
		t.Fatalf("party duel teleported at %d, want %d", teleportAt, teleportCountdown)
	}
	if want := []int{30, 20, 15, 10, 3, 2, 1, 0}; !slices.Equal(got, want) {
		t.Fatalf("party countdown = %v, want %v", got, want)
	}
}
