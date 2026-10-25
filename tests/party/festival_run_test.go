package party

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Dawn guide of the festival for level 31 or lower, its witch, and the
// seal stone its fee is paid in here.
const (
	dawnGuideID = 31127
	dawnWitchID = 31132
	blueStoneID = 6360
)

// festivalLinks are every festival command the fixture pages link.
const festivalLinks = `<a action="bypass -h npc_%objectId%_Festival 1">join</a>` +
	`<a action="bypass -h npc_%objectId%_Festival 2 6360">pay</a>` +
	`<a action="bypass -h npc_%objectId%_Festival 2 6361">pay green</a>` +
	`<a action="bypass -h npc_%objectId%_Festival 2 57">pay adena</a>` +
	`<a action="bypass -h npc_%objectId%_Festival 3">score</a>` +
	`<a action="bypass -h npc_%objectId%_Festival 8">harder</a>` +
	`<a action="bypass -h npc_%objectId%_Festival 9">leave</a>`

// festivalPages are the guide and festival pages, each linking every
// festival command.
func festivalPages() map[string]string {
	pages := map[string]string{"seven_signs/festival/dawn_guide.htm": "<html><body>guide" + festivalLinks + "</body></html>"}
	for _, p := range []string{"1", "2a", "2b", "2c", "2d", "2e", "2f", "3a", "3b", "3c", "3d", "8a", "8b", "8c"} {
		body := "page " + p
		switch p {
		case "1":
			body += " %blueStoneNeeded%/%greenStoneNeeded%/%redStoneNeeded%"
		case "2b":
			body += " %minFestivalPartyMembers%"
		}
		pages["seven_signs/festival/festival_"+p+".htm"] = "<html><body>" + body + festivalLinks + "</body></html>"
	}
	return pages
}

// festivalTemplates are the guide, the witch and every festival monster.
func festivalTemplates() *npc.Table {
	tmpls := []*npc.Template{gameservertest.FolkTemplate("FestivalGuide", dawnGuideID), gameservertest.FolkTemplate("FestivalGuide", dawnWitchID)}
	for id := 18009; id <= 18118; id++ {
		tmpls = append(tmpls, &npc.Template{
			ID: id, TemplateID: id, Type: "FestivalMonster", Name: "Offering", Level: 30, HPMax: 100,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20, CanMove: true,
		})
	}
	return npc.NewTable(tmpls)
}

// festivalItems are the shared item catalog plus the blue seal stone and
// the blood offering.
func festivalItems() *item.Table {
	stack := func(id int32, name string) *item.Template {
		return &item.Template{ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{}}
	}
	return item.NewTable(append(gameservertest.ItemTemplates().All(), stack(blueStoneID, "Blue Seal Stone"), stack(festival.BloodOfferingID, "Blood Offering")))
}

// festivalTimers is the festival's clock and timers, fired by hand: each
// fire advances the clock by the delay of the timer armed last and runs it.
type festivalTimers struct {
	mu      sync.Mutex
	now     time.Time
	delays  []time.Duration
	pending []func()
}

func (c *festivalTimers) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *festivalTimers) after(d time.Duration, fn func()) *time.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delays = append(c.delays, d)
	c.pending = append(c.pending, fn)
	return nil
}

// fire runs the timer armed last, which must be due after want.
func (c *festivalTimers) fire(t *testing.T, want time.Duration) {
	t.Helper()
	c.mu.Lock()
	n := len(c.pending)
	if n == 0 {
		c.mu.Unlock()
		t.Fatal("no festival timer armed")
	}
	d, fn := c.delays[n-1], c.pending[n-1]
	c.delays, c.pending = c.delays[:n-1], c.pending[:n-1]
	c.now = c.now.Add(d)
	c.mu.Unlock()
	if d != want {
		t.Fatalf("festival timer due after %v, want %v", d, want)
	}
	fn()
}

// festivalScores seeds a blank score for each festival and cabal of cycle
// 1, the cycle the fixture's Seven Signs status is in, under the scores
// given.
func festivalScores(t *testing.T, scores []festival.Score) func(*gamesql.FestivalStore) {
	return func(store *gamesql.FestivalStore) {
		rows := append([]festival.Score(nil), scores...)
		for id := range festival.Count {
			for _, cabal := range []sevensigns.Cabal{sevensigns.Dusk, sevensigns.Dawn} {
				rows = append(rows, festival.Score{FestivalID: id, Cabal: cabal, Cycle: 1})
			}
		}
		// A score given replaces the blank one saved after it.
		slices.Reverse(rows)
		if err := store.SaveScores(context.Background(), rows); err != nil {
			t.Fatalf("seed festival scores: %v", err)
		}
	}
}

// festivalRun is a party-ready group with a Dawn festival guide beside it,
// inside an oracle's peace zone.
type festivalRun struct {
	*group
	timers *festivalTimers
	guide  *npc.Folk
}

// bootFestival boots seats, the first holding blue stones of its own,
// beside the guide of the Dawn festival for level 31 or lower, the
// festival taking parties of minPlayers and more and its timers fired by
// hand.
func bootFestival(t *testing.T, seats []seat, blueStones int32, minPlayers int) *festivalRun {
	t.Helper()
	return bootFestivalWith(t, festivalSetup{seats: seats, blueStones: blueStones, minPlayers: minPlayers})
}

// festivalSetup is a festival fixture: its seats, the blue stones its
// first seat holds, the party minimum, the pages (festivalPages when nil),
// the scores seeded over cycle 1's blank ones and any further options.
type festivalSetup struct {
	seats      []seat
	blueStones int32
	minPlayers int
	pages      map[string]string
	scores     []festival.Score
	extra      []gameservertest.Option
}

func bootFestivalWith(t *testing.T, s festivalSetup) *festivalRun {
	t.Helper()
	timers := &festivalTimers{now: time.Now()}
	cfg := festival.DefaultConfig()
	cfg.MinPlayers = s.minPlayers
	oracle, err := zone.NewCuboid(-2000, 2000, -2000, 2000, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, oracle))
	pages := s.pages
	if pages == nil {
		pages = festivalPages()
	}
	seats := s.seats
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter(seats[0].name, seats[0].level, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithFestivalConfig(cfg),
		gameservertest.WithFestivalClock(timers.Now),
		gameservertest.WithFestivalTimer(timers.after),
		gameservertest.WithNPCs(festivalTemplates()),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithItemTemplates(festivalItems()),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithZones(zones),
		gameservertest.WithReuseDelays(3*time.Second, 0),
		gameservertest.WithFestivalSeed(festivalScores(t, s.scores)),
	}, s.extra...)...)
	leader := srv.SoleObjectID(t)
	if s.blueStones > 0 {
		srv.GiveItem(t, leader, blueStoneID, s.blueStones)
	}
	g := &group{srv: srv, players: []player{{c: srv.Client, id: leader, name: seats[0].name}}}
	for i, s := range seats[1:] {
		account := "member" + string(rune('a'+i))
		id := srv.SeedCharacterFor(t, account, s.name, s.level, 0).ID
		g.players = append(g.players, player{c: srv.DialClient(t, account, 1), id: id, name: s.name})
	}
	for _, p := range g.players {
		startInWorld(t, p.c)
	}
	x, y, z := srv.PlayerPosition(t, leader)
	guide := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("FestivalGuide", dawnGuideID), location.Location{X: x + 30, Y: y, Z: z})
	g.quiet(t)
	return &festivalRun{group: g, timers: timers, guide: guide}
}

// command has player i talk to the guide, then send it "Festival "+n, and
// returns the answer.
func (f *festivalRun) command(t *testing.T, i int, n string) [][]byte {
	t.Helper()
	return f.commandAt(t, i, f.guide, n)
}

// commandAt is command at guide.
func (f *festivalRun) commandAt(t *testing.T, i int, guide *npc.Folk, n string) [][]byte {
	t.Helper()
	p := f.players[i]
	x, y, z := f.srv.PlayerPosition(t, p.id)
	// A first click only selects the guide; the next one talks.
	opened := false
	for range 2 {
		p.c.Send(encodeAction(guide.ObjectID(), x, y, z))
		if _, opened = firstFrame(drainFrames(t, p.c), serverpackets.OpcodeNpcHtmlMessage); opened {
			break
		}
	}
	if !opened {
		t.Fatalf("%s: talking to the guide opened no page", p.name)
	}
	p.c.Send(encodeBypass(guide, "Festival "+n))
	return drainFrames(t, p.c)
}

// festivalPage is the festival page an answer opens, its links cut.
func festivalPage(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstFrame(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("answer %x opens no page", opcodes(frames))
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	html := r.ReadString()
	html, _, _ = strings.Cut(html, "<a ")
	return strings.TrimPrefix(html, "<html><body>")
}

// near reports whether (x, y) lies within r of (cx, cy) on each axis.
func near(x, y, cx, cy, r int) bool {
	return x >= cx-r && x <= cx+r && y >= cy-r && y <= cy+r
}

// texts are the plain-text system messages among frames.
func texts(frames [][]byte) []string {
	var out []string
	for _, m := range systemMessages(frames) {
		if m.id == serverpackets.SystemMessageS1 && len(m.params) == 1 {
			out = append(out, m.params[0].(string))
		}
	}
	return out
}

// sayings are the NpcSay and CreatureSay lines among frames.
func sayings(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeNpcSay:
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			r.ReadInt32()
			r.ReadInt32()
			out = append(out, r.ReadString())
		case serverpackets.OpcodeCreatureSay:
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			r.ReadInt32()
			name := r.ReadString()
			out = append(out, name+": "+r.ReadString())
		}
	}
	return out
}

// A party of two signs up for the Dawn festival for level 31 or lower,
// runs it and registers its score:
//   - signing up is refused outside a party (2b, naming the two-member
//     minimum) and to a member who does not lead it (2c); the leader sees
//     the fee (page 1), then pays 900 blue stones (2e), the festival's
//     bonus gaining 3 a stone;
//   - after the signup period the guides call the main event over the
//     oracle; the party is no longer there to hear it, taken into the
//     arena around its start, where neither may restart nor log out;
//   - the first wave starts, the witch saying so; a member's kill pays the
//     leader a blood offering;
//   - the archers come with the time left, then the chests;
//   - at the end both are taken back where they came from and told to
//     register their score, the monsters and the witch go and the witch's
//     shout reaches the oracle; the next cycle is due a minute later;
//   - the leader registers 5 points for its offering, the cycle's best of
//     Dawn's festival 0 naming the party and Dawn gaining the festival's 60
//     points.
func TestFestivalSignupRunAndScore(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Member", 20}}, 1000, 2)
	leader, member := f.players[0], f.players[1]

	if got := festivalPage(t, f.command(t, 0, "1")); got != "page 2b 2" {
		t.Fatalf("partyless signup = %q, want page 2b 2", got)
	}
	f.quiet(t)
	f.invite(t, 0, 1, 0)
	if got := festivalPage(t, f.command(t, 1, "1")); got != "page 2c" {
		t.Fatalf("member signup = %q, want page 2c", got)
	}
	if got := festivalPage(t, f.command(t, 0, "1")); got != "page 1 900/540/270" {
		t.Fatalf("leader signup = %q, want the fee page", got)
	}
	answer := f.command(t, 0, "2 6360")
	requireMessage(t, "leader", answer, serverpackets.SystemMessageS2S1Disappeared, int32(blueStoneID), int32(900))
	if got := festivalPage(t, answer); got != "page 2e" {
		t.Fatalf("payment = %q, want page 2e", got)
	}
	for _, p := range f.players {
		if !f.srv.Festival.IsParticipant(p.id) {
			t.Fatalf("%s is no participant after the payment", p.name)
		}
	}
	ctx := context.Background()
	if err := f.srv.Festival.SaveStatus(ctx); err != nil {
		t.Fatal(err)
	}
	var bonus int
	if err := f.srv.DB.QueryRow(`SELECT accumulated_bonus0 FROM seven_signs_status WHERE id = 0`).Scan(&bonus); err != nil || bonus != 2700 {
		t.Fatalf("festival 0 bonus = %d (%v), want 900 x 3", bonus, err)
	}
	origins := map[int32][2]int{}
	for _, p := range f.players {
		x, y, _ := f.srv.PlayerPosition(t, p.id)
		origins[p.id] = [2]int{x, y}
	}
	f.quiet(t)

	f.timers.fire(t, 2*time.Minute)  // the first cycle begins
	f.timers.fire(t, 19*time.Minute) // the signup period ends
	for _, p := range f.players {
		frames := drainFrames(t, p.c)
		if _, ok := firstFrame(frames, serverpackets.OpcodeTeleportToLocation); !ok {
			t.Fatalf("%s frames %x, want a teleport into the arena", p.name, opcodes(frames))
		}
		if got := sayings(frames); len(got) != 0 {
			t.Fatalf("%s heard %q on the way to the arena", p.name, got)
		}
		x, y, _ := f.srv.PlayerPosition(t, p.id)
		if !near(x, y, -79187, 113186, 230+20) {
			t.Fatalf("%s at (%d, %d), want around the arena's start", p.name, x, y)
		}
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)
	if !f.srv.Festival.Initialized() {
		t.Fatal("the festival did not begin")
	}

	leader.c.Send(encodeSingle(clientpackets.OpcodeRequestRestart))
	frames := drainFrames(t, leader.c)
	requireMessage(t, "restart", frames, serverpackets.SystemMessageNoRestartHere)
	if _, ok := firstFrame(frames, serverpackets.OpcodeRestartResponse); !ok {
		t.Fatalf("restart answer %x, want RestartResponse", opcodes(frames))
	}
	member.c.Send(encodeSingle(clientpackets.OpcodeLogout))
	frames = drainFrames(t, member.c)
	requireMessage(t, "logout", frames, serverpackets.SystemMessageNoLogoutHere)
	if _, ok := firstFrame(frames, serverpackets.OpcodeActionFailed); !ok {
		t.Fatalf("logout answer %x, want ActionFailed", opcodes(frames))
	}

	f.timers.fire(t, 2*time.Minute) // the first wave
	for _, p := range f.players {
		if got := sayings(drainFrames(t, p.c)); !contains(got, "The main event is now starting.") {
			t.Fatalf("%s heard %q, want the witch's start", p.name, got)
		}
	}
	var monster *npc.Hostile
	for _, obj := range f.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.FestivalMonster() {
			monster = h
			break
		}
	}
	if monster == nil {
		t.Fatal("no festival monster in the arena")
	}
	if !monster.TakeDamage(1000, f.combatant(t, 1)) {
		t.Fatal("the member's hit did not kill the monster")
	}
	requireMessage(t, "leader", drainFrames(t, leader.c), serverpackets.SystemMessageYouPickedUpS1, int32(festival.BloodOfferingID))

	f.timers.fire(t, 3*time.Minute) // the first rush
	f.timers.fire(t, 4*time.Minute) // the archers
	if got := sayings(drainFrames(t, leader.c)); !contains(got, "The Festival of Darkness will end in 9 minute(s).") {
		t.Fatalf("leader heard %q, want the time left", got)
	}
	f.timers.fire(t, 3*time.Minute) // the second rush
	f.timers.fire(t, 3*time.Minute) // the chests
	if got := sayings(drainFrames(t, leader.c)); !contains(got, "The chests have spawned! Be quick, the festival will end soon.") {
		t.Fatalf("leader heard %q, want the chests", got)
	}
	f.quiet(t)

	f.timers.fire(t, 3*time.Minute) // the end
	for _, p := range f.players {
		frames := drainFrames(t, p.c)
		want := []string{"You have been removed from the festival arena.", "The festival has ended. Your party leader must now register your score before the next festival takes place."}
		if got := texts(frames); !equalStrings(got, want) {
			t.Fatalf("%s told %q, want %q", p.name, got, want)
		}
		x, y, _ := f.srv.PlayerPosition(t, p.id)
		if o := origins[p.id]; !near(x, y, o[0], o[1], 20) {
			t.Fatalf("%s at (%d, %d), want back near (%d, %d)", p.name, x, y, o[0], o[1])
		}
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)
	if f.srv.Festival.Initialized() || f.srv.Festival.IsParticipant(leader.id) {
		t.Fatal("the festival still holds the party")
	}
	for _, obj := range f.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.FestivalMonster() {
			t.Fatalf("festival monster %d left in the arena", h.ObjectID())
		}
		if fo, ok := obj.(*npc.Folk); ok && fo.NpcID() == dawnWitchID {
			t.Fatal("the witch stayed in the arena")
		}
	}
	f.timers.mu.Lock()
	next := f.timers.delays
	f.timers.mu.Unlock()
	if len(next) != 1 || next[0] != time.Minute {
		t.Fatalf("next cycle armed after %v, want a minute", next)
	}

	answer = f.command(t, 0, "3")
	requireMessage(t, "score", answer, serverpackets.SystemMessageContribScoreIncreasedS1, int32(5))
	if got := festivalPage(t, answer); got != "page 3c" {
		t.Fatalf("score registration = %q, want page 3c", got)
	}
	var score, dawnScore int
	var members string
	if err := f.srv.DB.QueryRow(`SELECT score, members FROM seven_signs_festival WHERE festivalId = 0 AND cabal = 'DAWN' AND cycle = ?`, f.srv.SevenSigns.CurrentCycle()).Scan(&score, &members); err != nil {
		t.Fatal(err)
	}
	if score != 5 || members != "Leader,Member" {
		t.Fatalf("saved score = %d by %q, want 5 by Leader,Member", score, members)
	}
	if err := f.srv.DB.QueryRow(`SELECT dawn_festival_score FROM seven_signs_status WHERE id = 0`).Scan(&dawnScore); err != nil || dawnScore != 60 {
		t.Fatalf("dawn festival score = %d (%v), want 60", dawnScore, err)
	}
	if got := texts(f.command(t, 0, "3")); !equalStrings(got, []string{"You do not have any blood offerings to contribute."}) {
		t.Fatalf("second registration told %q, want no offerings", got)
	}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
