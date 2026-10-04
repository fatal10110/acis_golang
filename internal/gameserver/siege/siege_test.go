package siege

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TestNextDate pins Siege.setNextSiegeDate: from the stored date, or from
// now once it has passed, the castle's siege day of that week (Sunday for
// castles 3, 4, 6 and 7, Saturday for the others; a Calendar week starts on
// Sunday), plus two weeks, at 18:00. The vectors follow
// java.util.Calendar's lenient DAY_OF_WEEK resolution, including the week
// that crosses a year.
func TestNextDate(t *testing.T) {
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }
	now := at(2026, time.October, 7, 12) // a Wednesday
	for _, tc := range []struct {
		castle    int
		prev, now time.Time
		want      time.Time
	}{
		{1, at(2026, time.September, 1, 18), now, at(2026, time.October, 24, 18)},
		{3, at(2026, time.September, 1, 18), now, at(2026, time.October, 18, 18)},
		{1, at(2026, time.October, 24, 18), now, at(2026, time.November, 7, 18)},
		{7, at(2026, time.October, 25, 18), now, at(2026, time.November, 8, 18)},
		{5, time.UnixMilli(0), at(2026, time.December, 30, 9), at(2027, time.January, 16, 18)},
		{4, time.UnixMilli(0), at(2026, time.December, 30, 9), at(2027, time.January, 10, 18)},
		// A siege day itself: the same day two weeks on.
		{2, at(2026, time.October, 10, 18), at(2026, time.October, 10, 20), at(2026, time.October, 24, 18)},
	} {
		if got := NextDate(tc.castle, tc.prev, tc.now); !got.Equal(tc.want) {
			t.Errorf("NextDate(%d, %v, %v) = %v, want %v", tc.castle, tc.prev, tc.now, got, tc.want)
		}
	}
}

// The test clans. lords owns Gludio (1) in alliance 100 with kings; band1
// and band2 share alliance 200.
const (
	lords  int32 = 0x10000001
	rivals int32 = 0x10000002
	kings  int32 = 0x10000003
	weak   int32 = 0x10000004
	band1  int32 = 0x10000005
	band2  int32 = 0x10000006
	others int32 = 0x10000007
)

func testClans() *clan.Table {
	table := clan.NewTable()
	table.Restore(clan.Snapshot{Clans: []clan.Row{
		{ID: lords, Name: "Lords", Level: 5, CastleID: 1, AllyID: 100},
		{ID: rivals, Name: "Rivals", Level: 5},
		{ID: kings, Name: "Kings", Level: 5, AllyID: 100},
		{ID: weak, Name: "Weak", Level: 3},
		{ID: band1, Name: "BandOne", Level: 4, AllyID: 200},
		{ID: band2, Name: "BandTwo", Level: 4, AllyID: 200},
		{ID: others, Name: "Others", Level: 6},
	}}, time.Now(), 1)
	return table
}

// testCastles is Gludio (1) and Dion (2), sieged on Saturdays, and Giran
// (3), sieged on Sundays.
func testCastles(t *testing.T) *castledata.Table {
	t.Helper()
	var all []*castledata.Castle
	for _, a := range []castledata.CastleAttrs{
		{ID: 1, Alias: "gludio_castle", Name: "Gludio Castle"},
		{ID: 2, Alias: "dion_castle", Name: "Dion Castle"},
		{ID: 3, Alias: "giran_castle", Name: "Giran Castle"},
	} {
		c, err := castledata.NewCastle(a, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, c)
	}
	table, err := castledata.NewTable(all)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// recordingStore records each siege_clans write as one line.
type recordingStore struct {
	mu    sync.Mutex
	rows  []ClanRow
	lines []string
}

func (s *recordingStore) add(format string, args ...any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, fmt.Sprintf(format, args...))
	return nil
}

func (s *recordingStore) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.lines
	s.lines = nil
	return out
}

func (s *recordingStore) LoadClans(context.Context) ([]ClanRow, error) { return s.rows, nil }

func (s *recordingStore) SaveClan(_ context.Context, castleID, clanID int32, side Side) error {
	return s.add("save %d %#x %s", castleID, clanID, side)
}

func (s *recordingStore) DeleteClan(_ context.Context, castleID, clanID int32) error {
	return s.add("delete %d %#x", castleID, clanID)
}

func (s *recordingStore) DeleteClans(_ context.Context, castleID int32) error {
	return s.add("delete all %d", castleID)
}

func (s *recordingStore) DeletePending(_ context.Context, castleID int32) error {
	return s.add("delete pending %d", castleID)
}

// recordingNotifier records each notice as one line.
type recordingNotifier struct {
	mu    sync.Mutex
	lines []string
}

func (n *recordingNotifier) add(format string, args ...any) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lines = append(n.lines, fmt.Sprintf(format, args...))
}

func (n *recordingNotifier) take() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.lines
	n.lines = nil
	return out
}

func names(clans []*clan.Clan) string {
	out := make([]string, len(clans))
	for i, cl := range clans {
		out[i] = cl.Name()
	}
	return strings.Join(out, ",")
}

func msg(m Message) string {
	s := fmt.Sprint(m.ID)
	if m.Text != "" {
		s += " " + m.Text
	}
	if m.CastleID != 0 {
		s += fmt.Sprintf(" castle=%d", m.CastleID)
	}
	if m.HasNumber {
		s += fmt.Sprintf(" n=%d", m.Number)
	}
	return s
}

func (n *recordingNotifier) Announce(m Message)    { n.add("all %s", msg(m)) }
func (n *recordingNotifier) PlaySound(file string) { n.add("sound %s", file) }
func (n *recordingNotifier) TellClans(c []*clan.Clan, m Message) {
	n.add("tell [%s] %s", names(c), msg(m))
}

func (n *recordingNotifier) SetSiegeState(c []*clan.Clan, s State) {
	n.add("state [%s]=%d", names(c), s)
}

func (n *recordingNotifier) Reputation(cl *clan.Clan, points int, m Message) {
	n.add("reputation %s %+d %s", cl.Name(), points, msg(m))
}

func (n *recordingNotifier) CastleTaken(c *castle.Castle, owner, former *clan.Clan) {
	n.add("taken %d by %s from %s", c.ID, owner.Name(), former.Name())
}

// world is a siege engine over the test castles and clans.
type world struct {
	t       *testing.T
	clans   *clan.Table
	castles *castle.Manager
	store   *recordingStore
	field   *zone.Siege
	e       *Engine
	clock   *sim.Inline
	notes   *recordingNotifier
}

// The siege dates: Saturday 2026-10-10 and Sunday 2026-10-11, 18:00 UTC.
var (
	saturday = time.Date(2026, time.October, 10, 18, 0, 0, 0, time.UTC)
	sunday   = saturday.Add(24 * time.Hour)
)

// newWorld restores the engine with Gludio owned by lords, the Saturday
// date on Gludio and Dion and the Sunday date on Giran, and Gludio's
// battlefield a 1000-wide cube around the origin. rows are the stored
// registrations.
func newWorld(t *testing.T, cfg Config, rows ...ClanRow) *world {
	t.Helper()
	w := &world{t: t, clans: testClans(), store: &recordingStore{rows: rows}, notes: &recordingNotifier{}}
	w.castles = castle.NewManager(testCastles(t), w.clans, nil, nil, zerolog.Nop())
	w.castles.Restore([]castle.Row{
		{ID: 1, SiegeDate: saturday.UnixMilli()},
		{ID: 2, SiegeDate: saturday.UnixMilli()},
		{ID: 3, SiegeDate: sunday.UnixMilli()},
	}, []castle.Owner{{ClanID: lords, CastleID: 1}})
	form, err := zone.NewCuboid(-1000, 1000, -1000, 1000, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("castleId", 1)
	if w.field, err = zone.NewSiege(11, form, set); err != nil {
		t.Fatal(err)
	}
	w.e = New(cfg, w.castles, w.clans, []*zone.Siege{w.field}, w.store, nil, zerolog.Nop())
	if err := w.e.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return w
}

// only moves every castle's siege date but id's 60 days on, so their
// calendars stay quiet.
func (w *world) only(id int) {
	for _, s := range w.e.All() {
		if s.Castle().ID != id {
			s.Castle().SetSiegeDate(saturday.AddDate(0, 0, 60).UnixMilli())
		}
	}
}

// start runs the calendars on an inline clock reading now.
func (w *world) start(now time.Time) {
	w.clock = sim.NewInline(now)
	w.e.Start(w.clock.NewQueue("sieges"), w.notes)
}

// advanceTo moves the clock to at, firing every step due by then.
func (w *world) advanceTo(at time.Time) {
	w.clock.Advance(at.Sub(w.clock.Now()))
}

func (w *world) siege(id int) *Siege {
	s, ok := w.e.Get(id)
	if !ok {
		w.t.Fatalf("no siege for castle %d", id)
	}
	return s
}

func (w *world) clan(id int32) *clan.Clan {
	cl, ok := w.clans.Get(id)
	if !ok {
		w.t.Fatalf("no clan %#x", id)
	}
	return cl
}

func expectLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s:\n got %q\nwant %q", what, got, want)
	}
}

// TestRegistration pins Siege.registerAttacker / registerDefender /
// checkIfCanRegister: each refusal in the reference order with its
// message, the alliance rules, the side caps (pending defenders count as
// defenders), and the siege_clans write of each accepted registration.
func TestRegistration(t *testing.T) {
	w := newWorld(t, Config{MinClanLevel: 4, MaxAttackers: 2, MaxDefenders: 3})
	gludio, dion := w.siege(1), w.siege(2)
	refuse := func(what string, gotMsg Message, refused bool, want Message) {
		t.Helper()
		if !refused || gotMsg != want {
			t.Fatalf("%s = %+v refused %t, want %+v", what, gotMsg, refused, want)
		}
	}
	accept := func(what string, gotMsg Message, refused bool) {
		t.Helper()
		if refused {
			t.Fatalf("%s refused with %+v", what, gotMsg)
		}
	}

	m, r := gludio.RegisterAttacker(w.clan(kings))
	refuse("an ally of the owner attacks", m, r, Message{ID: MsgCannotAttackAllianceCastle})
	m, r = gludio.RegisterAttacker(w.clan(weak))
	refuse("a level 3 clan attacks", m, r, Message{ID: MsgOnlyClanLevel4})
	m, r = gludio.RegisterDefender(w.clan(lords))
	refuse("the owner asks to defend", m, r, Message{ID: MsgOwnerAutomaticallyDefends})
	m, r = dion.RegisterAttacker(w.clan(lords))
	refuse("the owner of another castle attacks", m, r, Message{ID: MsgOwnerCannotJoinOther})
	m, r = dion.RegisterDefender(w.clan(rivals))
	refuse("a defender of a castle no clan holds", m, r, Message{ID: MsgDefenderSideFull})

	m, r = gludio.RegisterAttacker(w.clan(rivals))
	accept("rivals attack Gludio", m, r)
	m, r = gludio.RegisterAttacker(w.clan(rivals))
	refuse("rivals attack Gludio again", m, r, Message{ID: MsgAlreadyRequested})
	m, r = w.siege(3).RegisterAttacker(w.clan(rivals))
	refuse("rivals attack Giran too", m, r, Message{ID: MsgAlreadyRequested})

	m, r = gludio.RegisterAttacker(w.clan(band1))
	accept("band1 attacks Gludio", m, r)
	m, r = gludio.RegisterDefender(w.clan(band2))
	refuse("band2 defends against its ally", m, r, Message{ID: MsgCantAcceptAllyEnemy})
	m, r = gludio.RegisterAttacker(w.clan(others))
	refuse("a third attacker", m, r, Message{ID: MsgAttackerSideFull})

	m, r = gludio.RegisterDefender(w.clan(others))
	accept("others ask to defend", m, r)
	m, r = gludio.RegisterDefender(w.clan(kings))
	accept("kings ask to defend", m, r)
	if got := gludio.Side(kings); got != SidePending {
		t.Fatalf("kings side = %s, want PENDING", got)
	}
	// The owner and two pending clans fill the three defender places.
	m, r = gludio.RegisterDefender(w.clan(band2))
	refuse("band2 defends against its attacking ally", m, r, Message{ID: MsgCantAcceptAllyEnemy})
	gludio.Unregister(w.clan(band1))
	m, r = gludio.RegisterDefender(w.clan(band2))
	refuse("band2 defends a full side", m, r, Message{ID: MsgDefenderSideFull})

	// The lord approves kings: still three, so the approval holds back as
	// in Siege.registerClan, which counts the pending clan itself.
	gludio.ConfirmWaiting(w.clan(kings), true)
	if got := gludio.Side(kings); got != SidePending {
		t.Fatalf("kings side after a full approval = %s, want PENDING", got)
	}
	gludio.ConfirmWaiting(w.clan(others), false)
	gludio.ConfirmWaiting(w.clan(kings), true)
	if got := gludio.Side(kings); got != SideDefender {
		t.Fatalf("kings side after approval = %s, want DEFENDER", got)
	}
	gludio.Unregister(w.clan(lords))
	if got := gludio.Side(lords); got != SideOwner {
		t.Fatalf("the owner unregistered itself: side %s", got)
	}

	expectLines(t, "siege_clans writes", w.store.take(), []string{
		"save 1 0x10000002 ATTACKER",
		"save 1 0x10000005 ATTACKER",
		"save 1 0x10000007 PENDING",
		"save 1 0x10000003 PENDING",
		"delete 1 0x10000005",
		"delete 1 0x10000007",
		"save 1 0x10000003 DEFENDER",
	})
	if got := names(gludio.Attackers()); got != "Rivals" {
		t.Fatalf("attackers = %s", got)
	}
	var defenders []string
	for _, d := range gludio.Defenders() {
		defenders = append(defenders, d.Clan.Name()+"/"+d.Side.String())
	}
	if want := []string{"Lords/OWNER", "Kings/DEFENDER"}; !slices.Equal(defenders, want) {
		t.Fatalf("defenders = %v, want %v", defenders, want)
	}
	if !w.e.Registered(rivals) || w.e.Registered(others) {
		t.Fatal("Registered: want rivals only")
	}
	if !gludio.OnOppositeSides(rivals, kings) || gludio.OnOppositeSides(lords, kings) || gludio.OnOppositeSides(rivals, others) {
		t.Fatal("OnOppositeSides: want rivals against kings only")
	}
	if !gludio.CheckSides(kings, SideDefender, SideOwner) || gludio.CheckSides(kings, SideAttacker) || !gludio.CheckSides(rivals) {
		t.Fatal("CheckSides mismatch")
	}
}

// TestRestore pins the Siege constructor: the castle owner first, as
// OWNER, then the stored rows; a row of an unknown clan or castle is
// skipped.
func TestRestore(t *testing.T) {
	w := newWorld(t, DefaultConfig(),
		ClanRow{CastleID: 1, ClanID: rivals, Side: SideAttacker},
		ClanRow{CastleID: 1, ClanID: kings, Side: SidePending},
		ClanRow{CastleID: 1, ClanID: 0x7fff, Side: SideAttacker},
		ClanRow{CastleID: 9, ClanID: others, Side: SideAttacker},
	)
	gludio := w.siege(1)
	for id, want := range map[int32]Side{lords: SideOwner, rivals: SideAttacker, kings: SidePending, others: SideNone} {
		if got := gludio.Side(id); got != want {
			t.Errorf("side of %#x = %s, want %s", id, got, want)
		}
	}
	if w.e.Registered(others) {
		t.Fatal("a row of an unknown castle registered its clan")
	}
}

// TestBootWithPassedDate pins Siege.startAutoTask for a date already
// passed: the next date is set and announced, the registrations open,
// and nothing is scheduled.
func TestBootWithPassedDate(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	w.start(time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC))
	for _, s := range w.e.All() {
		if s.Status() != StatusRegistrationOpened || s.Castle().IsTimeRegistrationOver() {
			t.Fatalf("castle %d: status %d, registration over %t", s.Castle().ID, s.Status(), s.Castle().IsTimeRegistrationOver())
		}
	}
	if got, want := time.UnixMilli(w.siege(1).Date()).UTC(), time.Date(2026, time.October, 31, 18, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Gludio date = %v, want %v", got, want)
	}
	if got, want := time.UnixMilli(w.siege(3).Date()).UTC(), time.Date(2026, time.October, 25, 18, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Giran date = %v, want %v", got, want)
	}
	expectLines(t, "notices", w.notes.take(), []string{"all 292 castle=1", "all 292 castle=2", "all 292 castle=3"})
	if at, ok := w.clock.NextTimer(); ok {
		t.Fatalf("a step is armed for %v", at)
	}
}

// TestSiegeLifecycle drives Gludio's siege from boot to its end, as
// Siege.siegeStart, startSiege, processSiegeTimer and endSiege run it: the
// registrations close a day before, dropping the pending defenders; the
// siege starts at its date and turns its battlefield on; the clock tells
// both sides the time left; the end announces the held castle, pays the
// former owner 500 reputation, clears the siege states and every
// registration, sets the next date and turns the battlefield off.
func TestSiegeLifecycle(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	gludio := w.siege(1)
	if _, refused := gludio.RegisterAttacker(w.clan(rivals)); refused {
		t.Fatal("rivals refused")
	}
	if _, refused := gludio.RegisterDefender(w.clan(kings)); refused {
		t.Fatal("kings refused")
	}
	w.store.take()
	w.only(1)
	w.start(saturday.Add(-54*time.Hour - time.Second))
	w.e.RecordKill(rivals, lords)
	w.e.RecordKill(others, rivals)

	w.advanceTo(saturday.Add(-24*time.Hour - time.Millisecond))
	if gludio.RegistrationOver() || gludio.Castle().IsTimeRegistrationOver() {
		t.Fatal("registrations closed early")
	}
	w.advanceTo(saturday.Add(-24 * time.Hour))
	if gludio.Status() != StatusRegistrationOver || !gludio.Castle().IsTimeRegistrationOver() {
		t.Fatal("registrations still open a day before the siege")
	}
	if got := gludio.Side(kings); got != SideNone {
		t.Fatalf("pending kings kept: %s", got)
	}
	if m, refused := gludio.RegisterAttacker(w.clan(others)); !refused || m != (Message{ID: MsgDeadlinePassed, CastleID: 1}) {
		t.Fatalf("late registration = %+v refused %t", m, refused)
	}
	expectLines(t, "registration end", w.notes.take(), []string{"all 293 castle=1"})
	expectLines(t, "registration end writes", w.store.take(), []string{"delete pending 1"})

	w.advanceTo(saturday.Add(-time.Millisecond))
	if gludio.InProgress() || w.field.Active() {
		t.Fatal("siege started early")
	}
	w.advanceTo(saturday)
	if !gludio.InProgress() || !w.field.Active() {
		t.Fatal("siege not under way at its date")
	}
	expectLines(t, "start", w.notes.take(), []string{
		"state [Rivals]=1",
		"state [Lords]=2",
		"all 711 castle=1",
		"sound systemmsg_e.17",
		"tell [Rivals] 1189",
	})
	if s, ok := w.e.ActiveAt(10, 10, 10); !ok || s != gludio {
		t.Fatal("ActiveAt inside the battlefield found no siege")
	}
	if _, ok := w.e.ActiveAt(5000, 0, 0); ok {
		t.Fatal("ActiveAt outside the battlefield found a siege")
	}

	w.advanceTo(saturday.Add(2*time.Hour - time.Millisecond))
	both := func(m string) string { return "tell [Rivals] " + m + "|tell [Lords] " + m }
	var want []string
	for _, m := range []string{"358 n=1", "359 n=30", "359 n=10", "359 n=5", "359 n=1"} {
		want = append(want, strings.Split(both(m), "|")...)
	}
	for n := 10; n >= 1; n-- {
		want = append(want, strings.Split(both(fmt.Sprintf("360 n=%d", n)), "|")...)
	}
	expectLines(t, "siege clock", w.notes.take(), want)

	w.advanceTo(saturday.Add(2 * time.Hour))
	if gludio.InProgress() || w.field.Active() {
		t.Fatal("siege still under way at its end")
	}
	expectLines(t, "end", w.notes.take(), []string{
		"all 712 castle=1",
		"sound systemmsg_e.18",
		"all 291 Lords castle=1",
		"reputation Lords +500 1773 n=500",
		"state [Rivals]=0",
		"state [Lords]=0",
		"all 292 castle=1",
	})
	expectLines(t, "end writes", w.store.take(), []string{"delete all 1"})
	if gludio.Side(rivals) != SideNone || gludio.Side(lords) != SideOwner {
		t.Fatal("registrations not reset to the owner")
	}
	if w.e.Kills(rivals) != 0 || w.e.Deaths(lords) != 0 || w.e.Deaths(rivals) != 0 {
		t.Fatal("the registered clans kept their siege counters")
	}
	if w.e.Kills(others) != 1 {
		t.Fatalf("an unregistered clan lost its siege kills: %d", w.e.Kills(others))
	}
	if got, want := time.UnixMilli(gludio.Date()).UTC(), saturday.AddDate(0, 0, 14); !got.Equal(want) {
		t.Fatalf("next date = %v, want %v", got, want)
	}
	if gludio.Status() != StatusRegistrationOpened || gludio.Castle().IsTimeRegistrationOver() {
		t.Fatal("registrations not reopened")
	}
}

// TestSiegeCancelled pins startSiege with no attacker: everyone is told
// the siege is cancelled (295 for an owned castle, 846 for one the NPCs
// hold) and the next date is set and its calendar armed. Booted an hour
// before, Gludio and Dion close their registrations without the 293
// notice (less than 13,600,000 ms are left); Giran, a day later, gives it
// at the same instant, its step armed first.
func TestSiegeCancelled(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	w.start(saturday.Add(-time.Hour))
	w.advanceTo(saturday)
	expectLines(t, "cancelled sieges", w.notes.take(), []string{
		"all 293 castle=3",
		"all 295 castle=1",
		"all 292 castle=1",
		"all 846 castle=2",
		"all 292 castle=2",
	})
	for _, id := range []int{1, 2} {
		s := w.siege(id)
		if s.InProgress() || !time.UnixMilli(s.Date()).Equal(saturday.AddDate(0, 0, 14)) {
			t.Fatalf("castle %d: in progress %t, date %v", id, s.InProgress(), time.UnixMilli(s.Date()))
		}
	}
	if _, ok := w.clock.NextTimer(); !ok {
		t.Fatal("no calendar step armed after the cancel")
	}
}

// TestMidVictory pins Castle.setOwner during a siege and Siege.midVictory
// / endSiege for a taken castle: the attackers are told their alliance is
// over, the former owner and defenders attack, the new owner owns and its
// attacking allies defend; at the end the former owner loses its castle
// items and 1000 reputation and the new owner gains 1000. As in the
// reference, a new owner in an alliance is itself among the attackers of
// that alliance switched to defend, so it ends on the defender side.
func TestMidVictory(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	gludio := w.siege(1)
	for _, id := range []int32{band1, band2, rivals} {
		if _, refused := gludio.RegisterAttacker(w.clan(id)); refused {
			t.Fatalf("%#x refused", id)
		}
	}
	if _, refused := gludio.RegisterDefender(w.clan(kings)); refused {
		t.Fatal("kings refused")
	}
	gludio.ConfirmWaiting(w.clan(kings), true)
	w.start(saturday.Add(-time.Second))
	gludio.Start()
	w.notes.take()

	former, _ := w.castles.SetOwner(gludio.Castle(), w.clan(band1))
	if former == nil || former.ID() != lords {
		t.Fatalf("former owner = %v", former)
	}
	gludio.MidVictory()
	for id, want := range map[int32]Side{band1: SideDefender, band2: SideDefender, rivals: SideAttacker, lords: SideAttacker, kings: SideAttacker} {
		if got := gludio.Side(id); got != want {
			t.Errorf("side of %#x = %s, want %s", id, got, want)
		}
	}
	expectLines(t, "mid-victory", w.notes.take(), []string{
		"tell [BandOne,BandTwo,Rivals] 1190",
		"state [Lords,Rivals,Kings]=1",
		"state [BandOne,BandTwo]=2",
	})

	gludio.End()
	expectLines(t, "end", w.notes.take(), []string{
		"all 712 castle=1",
		"sound systemmsg_e.18",
		"all 291 BandOne castle=1",
		"taken 1 by BandOne from Lords",
		"reputation Lords -1000 1772 n=1000",
		"reputation BandOne +1000 1773 n=1000",
		"state [Lords,Rivals,Kings]=0",
		"state [BandOne,BandTwo]=0",
		"all 292 castle=1",
	})
	if gludio.Side(band1) != SideOwner || gludio.Side(band2) != SideNone {
		t.Fatal("the new owner was not registered alone")
	}
}

// TestMidVictoryWithoutOwner pins midVictory after Castle.removeOwner: with
// no owner left nothing changes but the dropped owner registration.
func TestMidVictoryWithoutOwner(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	gludio := w.siege(1)
	if _, refused := gludio.RegisterAttacker(w.clan(rivals)); refused {
		t.Fatal("rivals refused")
	}
	w.start(saturday.Add(-time.Second))
	gludio.Start()
	w.notes.take()
	if _, ok := w.castles.RemoveOwner(gludio.Castle()); !ok {
		t.Fatal("no owner removed")
	}
	gludio.DropOwner(lords)
	gludio.MidVictory()
	if got := w.notes.take(); len(got) != 0 {
		t.Fatalf("mid-victory without owner noticed %q", got)
	}
	gludio.End()
	expectLines(t, "draw", w.notes.take()[:3], []string{"all 712 castle=1", "sound systemmsg_e.18", "all 856 castle=1"})
}
