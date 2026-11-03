package quest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q635"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// In the Dimensional Rift (Q635_InTheDimensionalRift) works on a party, the
// rooms of a level, the guardians' timers and the maker events they send,
// none of which a scenario has a line for (#4020, #4048): the party gate of
// a rift post, a room's watch, its jumps and its time limit, and the
// guardians' give-up and change-room links. These Boot tests play them; the
// scenarios play the keepers' dialogs and the post's pages of one character.
const (
	q635Name     = "Q635_InTheDimensionalRift"
	q635Path     = "quest.Q635_InTheDimensionalRift"
	q635Post     = 31488
	q635Guardian = 31865 // the guardian of room i is 31865+i
	q635Invader  = 21659
	q635Fragment = 7079
	q635Rooms    = 9
)

// Room i spans 400 by 400 from x = 20000+1000*i and holds its guardian at 60
// east and 20 south of that corner, its invader at 100 east. The post sends a
// party 30 east and 30 south of the guardian, within its reach, and a party
// put out of the rift stands near q635Escape.
func q635RoomX(i int) int { return 20000 + 1000*i }

var q635Escape = location.Location{X: -5000, Y: 20, Z: 30}

// q635Spawnlist is the nine rooms of one level, each an npcmaker of the
// teleport dungeon type with its guardian and one invader.
func q635Spawnlist() string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<list>\n")
	for i := range q635Rooms {
		x := q635RoomX(i)
		fmt.Fprintf(&b, "\t<territory name=\"room%d\" minZ=\"-20000\" maxZ=\"20000\"><node x=\"%d\" y=\"0\"/><node x=\"%d\" y=\"0\"/><node x=\"%d\" y=\"400\"/><node x=\"%d\" y=\"400\"/></territory>\n",
			i, x, x+400, x+400, x)
		fmt.Fprintf(&b, "\t<npcmaker name=\"dimension_gap_%02d\" territory=\"room%d\" maximumNpcs=\"6\">\n", i, i)
		fmt.Fprintf(&b, "\t\t<ai type=\"manage_teleport_dungeon\"><set name=\"manager_npc_name\" val=\"R%02d_INVADER_TELEPORT\"/><set name=\"room_index\" val=\"%02d\"/></ai>\n", i, i)
		fmt.Fprintf(&b, "\t\t<npc id=\"%d\" total=\"1\" pos=\"%d;20;30;0\" respawn=\"1min\"><ai>", q635Guardian+i, x+60)
		fmt.Fprintf(&b, "<set name=\"escape_x\" val=\"%d\"/><set name=\"escape_y\" val=\"%d\"/><set name=\"escape_z\" val=\"%d\"/>",
			q635Escape.X, q635Escape.Y, q635Escape.Z)
		for r := range q635Rooms {
			fmt.Fprintf(&b, "<set name=\"PosX%d\" val=\"%d\"/><set name=\"PosY%d\" val=\"50\"/>", r, q635RoomX(r)+90, r)
		}
		fmt.Fprintf(&b, "<set name=\"LevelName\" val=\"30level\"/><set name=\"RoomIndex\" val=\"%d\"/></ai></npc>\n", i)
		fmt.Fprintf(&b, "\t\t<npc id=\"%d\" total=\"1\" pos=\"%d;20;30;0\" respawn=\"90sec\"/>\n", q635Invader, x+100)
		b.WriteString("\t</npcmaker>\n")
	}
	b.WriteString("</list>\n")
	return b.String()
}

// q635Rand pins the script's draws by their bound: 8 is the pick of a room
// (the draw it is told), 7 the pick among the seven free rooms a change of
// room has, 100 the Anakazel roll; every other draw is 0.
type q635Rand struct{ room, free, anakazel atomic.Int32 }

func (r *q635Rand) rand(n int) int {
	switch n {
	case 8:
		return int(r.room.Load())
	case 7:
		return int(r.free.Load())
	case 100:
		return int(r.anakazel.Load())
	}
	return 0
}

// q635Spec is how a world differs from the default: a party of Leader and
// the mates, each with 60 dimension fragments and the quest started.
type q635Spec struct {
	mates        int
	rules        *script.EventRules
	questless    string
	fragmentless string
	returning    string // a member sent out of the rift an instant ago
	recruiting   bool   // the Seven Signs are in the recruiting period, not the competition
	extra        []gameservertest.Option
}

type q635World struct {
	srv     *gameservertest.Server
	ids     map[string]int32
	clients map[int32]*testsupport.ScriptedClient
	at      location.Location
	roles   map[int32]string
	rand    *q635Rand
	post    *npc.Folk
	start   time.Time
	elapsed atomic.Int64
}

// now is the in-game clock's wall clock: the boot's time plus what the
// world advanced since, so the game ticks the guardians count move along.
func (w *q635World) now() time.Time { return w.start.Add(time.Duration(w.elapsed.Load())) }

// step moves the world's clock d on.
func (w *q635World) step(t *testing.T, d time.Duration) {
	t.Helper()
	w.elapsed.Add(int64(d))
	w.srv.Advance(t, d)
}

func q635Rules(f func(*script.EventRules)) *script.EventRules {
	r := script.DefaultEventRules()
	f(&r)
	return &r
}

func q635Boot(t *testing.T, spec q635Spec) *q635World {
	t.Helper()
	w := &q635World{rand: &q635Rand{}, roles: map[int32]string{}, start: time.Now()}
	pages := shippedQuestPages(t, q635Name)

	postTmpl := gameservertest.FolkTemplate("Folk", q635Post)
	postTmpl.AIParams = npc.AIParams{"LevelName": "30level"}
	for i := range q635Rooms {
		postTmpl.AIParams["PosX"+strconv.Itoa(i)] = strconv.Itoa(q635RoomX(i) + 90)
		postTmpl.AIParams["PosY"+strconv.Itoa(i)] = "50"
	}
	templates := []*npc.Template{postTmpl, proofMonster(q635Invader)}
	kinds := map[int32]script.NPCKind{q635Post: script.KindFolk, q635Invader: script.KindHostile}
	for i := range q635Rooms {
		guardian := gameservertest.FolkTemplate("Folk", q635Guardian+i)
		guardian.Alias = fmt.Sprintf("r%02d_invader_teleport", i)
		templates = append(templates, guardian)
		kinds[int32(q635Guardian+i)] = script.KindFolk
	}

	dir := t.TempDir()
	writeFile(t, dir, "rift.xml", q635Spawnlist())
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	makers := script.MakerCatalog{"manage_teleport_dungeon": catalog.Makers()["makers_2"]["manage_teleport_dungeon"]}

	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithMakers(makers),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q635Path}}, script.Catalog{q635Path: q635.New}),
		gameservertest.WithScriptRand(w.rand.rand),
		gameservertest.WithGameClockNow(w.now),
		x1Items(append(q635.New().Items, q635Fragment)),
	}
	if spec.rules != nil {
		opts = append(opts, gameservertest.WithEventRules(*spec.rules))
	}
	// The Seven Signs run on a pinned Wednesday noon, away from the weekly
	// period change, in the period the test asks for.
	period := sevensigns.Competition
	if spec.recruiting {
		period = sevensigns.Recruiting
	}
	opts = append(opts,
		gameservertest.WithSevenSignsClock(func() time.Time { return time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC) }),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Errorf("load seven signs status: found=%v err=%v", found, err)
				return
			}
			row.Period = period
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Errorf("seed seven signs status: %v", err)
			}
		}))
	opts = append(opts, spec.extra...)
	srv := gameservertest.Boot(t, opts...)
	w.srv = srv

	seed := func(id int32, name string) {
		if name != spec.questless {
			insertJournal(t, srv.DB,
				journalRow{id, q635Name, "<state>", val("STARTED")},
				journalRow{id, q635Name, "<cond>", val("1")},
			)
		}
		if name == spec.returning {
			insertJournal(t, srv.DB,
				journalRow{id, q635Name, "memoState-0", val("2")},
				journalRow{id, q635Name, "memoState-1", val("0")},
			)
		}
		if name != spec.fragmentless {
			srv.GiveItem(t, id, q635Fragment, 60)
		}
	}
	var mates []partyMate
	for i := range spec.mates {
		mates = append(mates, partyMate{"mate" + strconv.Itoa(i+1), "Mate" + strconv.Itoa(i+1), 75})
	}
	if len(mates) == 0 {
		leader := srv.SoleObjectID(t)
		seed(leader, "Leader")
		w.ids = map[string]int32{"Leader": leader}
		enterWorld(t, srv)
		w.clients = map[int32]*testsupport.ScriptedClient{leader: srv.Client}
	} else {
		w.ids, w.clients = formParty(t, srv, "Leader", mates, seed)
	}
	x, y, z := srv.PlayerPosition(t, w.ids["Leader"])
	w.at = location.Location{X: x, Y: y, Z: z}
	w.post = srv.SpawnFolkNPCAt(t, postTmpl, w.at)
	srv.RunCreated(t, w.post)
	w.step(t, time.Millisecond)
	w.roles[w.post.ObjectID()] = "post"
	for _, c := range w.clients {
		srv.ReadQueued(t, c)
	}
	return w
}

// writeFile writes the file name of dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// find returns the NPCs of id in the world.
func (w *q635World) find(id int) []attackable.Combatant {
	var out []attackable.Combatant
	for _, obj := range w.srv.State.Objects() {
		c, ok := obj.(attackable.Combatant)
		if !ok {
			continue
		}
		if n := script.NPCOf(c); n != nil && n.NpcID() == int32(id) {
			out = append(out, c)
		}
	}
	return out
}

func (w *q635World) guardian(t *testing.T, room int) attackable.Combatant {
	t.Helper()
	got := w.find(q635Guardian + room)
	if len(got) != 1 {
		t.Fatalf("guardians of room %d in the world = %d, want 1", room, len(got))
	}
	return got[0]
}

// run runs fn as who's script.
func (w *q635World) run(t *testing.T, who string, fn func(s *script.Script, p *script.Player)) {
	t.Helper()
	if !w.srv.RunScript(t, w.ids[who], q635Name, fn) {
		t.Fatal("the script panicked")
	}
}

// lines renders what the server queued to who's client.
func (w *q635World) lines(t *testing.T, who string) []string {
	t.Helper()
	var out []string
	for _, f := range w.srv.ReadQueued(t, w.clients[w.ids[who]]) {
		if deferredUpdate(f) {
			continue
		}
		if line, err := q635Packet(f, w.roles); err == nil {
			out = append(out, line)
		}
	}
	return out
}

// arrive has every client confirm the teleport it was sent on, which a
// character must for the server to take its next request, and returns what
// Leader's client was sent before.
func (w *q635World) arrive(t *testing.T) [][]byte {
	t.Helper()
	var leader [][]byte
	w.srv.Settle(t)
	for id, c := range w.clients {
		frames := w.srv.ReadQueued(t, c)
		if id == w.ids["Leader"] {
			leader = frames
		}
		c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
		w.srv.ReadQueued(t, c)
	}
	return leader
}

// tick moves the clock the 10 seconds of the guardians' watch and returns
// what Leader's client was sent in them.
func (w *q635World) tick(t *testing.T) [][]byte {
	t.Helper()
	w.step(t, 10*time.Second)
	return w.arrive(t)
}

// talk has who click the NPC obj twice, which opens its first page, and
// follow the links of events in turn, and returns what the last answer sent.
func (w *q635World) talk(t *testing.T, who string, obj int32, events ...string) []string {
	t.Helper()
	c := w.clients[w.ids[who]]
	for range 2 {
		c.Send(encodeTutorialAction(obj, w.at))
		w.srv.ReadQueued(t, c)
		w.step(t, 500*time.Millisecond)
	}
	var out []string
	for _, ev := range events {
		c.Send(encodeBypass("Quest " + q635Name + " " + ev))
		out = w.lines(t, who)
		w.step(t, 500*time.Millisecond)
	}
	return out
}

// room is the room of 0 to 8 a position stands in, -1 for none.
func q635RoomAt(x, y int) int {
	for i := range q635Rooms {
		if x >= q635RoomX(i) && x <= q635RoomX(i)+400 && y >= 0 && y <= 400 {
			return i
		}
	}
	return -1
}

func (w *q635World) roomOf(t *testing.T, who string) int {
	t.Helper()
	x, y, _ := w.srv.PlayerPosition(t, w.ids[who])
	return q635RoomAt(x, y)
}

// out reports whether who stands within 100 of the place the rift puts
// players out at.
func (w *q635World) out(t *testing.T, who string) bool {
	t.Helper()
	x, y, _ := w.srv.PlayerPosition(t, w.ids[who])
	return x >= q635Escape.X-100 && x <= q635Escape.X+100 && y >= q635Escape.Y-100 && y <= q635Escape.Y+100
}

// back puts who back beside the post.
func (w *q635World) back(t *testing.T, who string) {
	t.Helper()
	w.run(t, who, func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: w.at.X, Y: w.at.Y, Z: w.at.Z}, 0)
	})
	w.srv.Settle(t)
	w.arrive(t)
}

// memo reads who's memo state slot of the quest.
func (w *q635World) memo(t *testing.T, who string, slot int32) int32 {
	t.Helper()
	var got int32
	w.run(t, who, func(s *script.Script, p *script.Player) { got = s.MemoStateEx(p, 635, slot) })
	return got
}

func (w *q635World) fragments(t *testing.T, who string) int {
	t.Helper()
	return w.srv.PlayerItemCount(t, w.ids[who], q635Fragment)
}

// page is the page file of the quest with every "<?name?>" filled.
func q635Page(t *testing.T, file, name string) string {
	t.Helper()
	return strings.ReplaceAll(datapackPage(t, "script/quest/"+q635Name+"/"+file), "<?name?>", name)
}

// enter has who ask the post to take the party in, from its first page, and
// returns the lines of the answer.
func (w *q635World) enter(t *testing.T, who string) []string {
	t.Helper()
	got := w.talk(t, who, w.post.ObjectID(), "rift_watcher_05.htm")
	w.arrive(t)
	return got
}

// advance moves the clock d in steps of 10 seconds, the pace of the
// guardians' watch.
func (w *q635World) advance(t *testing.T, d time.Duration) {
	t.Helper()
	for passed := time.Duration(0); passed < d; passed += 10 * time.Second {
		w.tick(t)
	}
}

// q635Packet renders a frame the way the goldens do.
func q635Packet(f []byte, roles map[int32]string) (string, error) {
	return scriptcontract.Packet(f, roles)
}

func opcodeCount(frames [][]byte, opcode byte) int {
	n := 0
	for _, f := range frames {
		if len(f) > 0 && f[0] == opcode {
			n++
		}
	}
	return n
}

func encodeOustPartyMember(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestOustPartyMember)
	w.WriteString(name)
	return w.Bytes()
}

// TestQ635WatcherTurnsAwayWhoCannotEnter: a rift post answers a character
// alone, or in a party smaller than the setting asks for, with its party
// page, a party member who is not the leader with the page naming the
// leader, a party with a member who has not the quest with the page naming
// him, and a member short of the fragments with the page naming him; no one
// moves and nothing is paid.
func TestQ635WatcherTurnsAwayWhoCannotEnter(t *testing.T) {
	t.Parallel()
	three := q635Rules(func(r *script.EventRules) { r.Rift.MinParty = 3 })
	cases := []struct {
		name   string
		spec   q635Spec
		who    string
		page   string
		member string
	}{
		{"alone", q635Spec{}, "Leader", "rift_watcher_01.htm", ""},
		{"too small for the setting", q635Spec{mates: 1, rules: three}, "Leader", "rift_watcher_01.htm", ""},
		{"not the leader", q635Spec{mates: 1}, "Mate1", "rift_watcher_02.htm", "Leader"},
		{"no quest", q635Spec{mates: 1, questless: "Mate1"}, "Leader", "rift_watcher_03a.htm", "Mate1"},
		{"no fragments", q635Spec{mates: 1, fragmentless: "Mate1"}, "Leader", "rift_watcher_03.htm", "Mate1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, c.spec)
			got := w.enter(t, c.who)
			requireLine(t, got, htmlLine("post", q635Page(t, c.page, c.member)))
			for who := range w.ids {
				if r := w.roomOf(t, who); r != -1 {
					t.Errorf("%s stands in room %d", who, r)
				}
				if n := w.fragments(t, who); n != 60 && who != c.spec.fragmentless {
					t.Errorf("%s holds %d fragments, want 60", who, n)
				}
			}
		})
	}
}

// TestQ635PartyEntersARoom: a party of Leader and two mates pays the
// fragments of the post, as the setting says (7 where the shipped one is
// 21), the room the draw picks is taken, every member stands in it with the
// tick it came and Leader alone as the one who may change room. Asking again
// finds the room taken by the party, and a party of the setting's size with
// fewer than three free rooms is turned away.
func TestQ635PartyEntersARoom(t *testing.T) {
	t.Parallel()
	w := q635Boot(t, q635Spec{mates: 2, rules: q635Rules(func(r *script.EventRules) { r.Rift.Costs[0] = 7 })})
	w.rand.room.Store(5)
	got := w.enter(t, "Leader")
	requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_05.htm", "")))
	for who := range w.ids {
		if r := w.roomOf(t, who); r != 5 {
			t.Errorf("%s stands in room %d, want 5", who, r)
		}
		if n := w.fragments(t, who); n != 53 {
			t.Errorf("%s holds %d fragments, want 53", who, n)
		}
		if s := w.memo(t, who, 0); s < 0 {
			t.Errorf("%s memo state = %d, want the tick it came", who, s)
		}
		want := int32(0)
		if who == "Leader" {
			want = 1
		}
		if s := w.memo(t, who, 1); s != want {
			t.Errorf("%s memo state 1 = %d, want %d", who, s, want)
		}
	}

	w.back(t, "Leader")
	got = w.enter(t, "Leader")
	requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_08.htm", "")))
	if n := w.fragments(t, "Leader"); n != 53 {
		t.Errorf("Leader holds %d fragments after asking again, want 53", n)
	}
}

// TestQ635ReturningMemberGoesInFree: a member with no fragments, sent out of
// the rift within the hour, goes in with the party and pays nothing.
func TestQ635ReturningMemberGoesInFree(t *testing.T) {
	t.Parallel()
	w := q635Boot(t, q635Spec{mates: 1, fragmentless: "Mate1", returning: "Mate1"})
	w.rand.room.Store(3)
	w.enter(t, "Leader")
	if r := w.roomOf(t, "Mate1"); r != 3 {
		t.Errorf("Mate1 stands in room %d, want 3", r)
	}
	if n := w.fragments(t, "Leader"); n != 39 {
		t.Errorf("Leader holds %d fragments, want 39", n)
	}
}

// TestQ635WatchOustsABrokenParty: once a party is in a room, its guardian
// opens the room's monsters, and puts everyone out of the rift, closing the
// room, when a member is expelled from the party or the leader leaves the
// room's territory.
func TestQ635WatchOustsABrokenParty(t *testing.T) {
	t.Parallel()
	for _, breakIt := range []string{"expel a member", "leader leaves"} {
		t.Run(breakIt, func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, q635Spec{mates: 2})
			w.rand.room.Store(5)
			w.enter(t, "Leader")
			if n := len(w.find(q635Invader)); n != 0 {
				t.Fatalf("invaders before the watch = %d, want 0", n)
			}
			w.advance(t, 10*time.Second)
			if n := len(w.find(q635Invader)); n != 1 {
				t.Fatalf("invaders in the opened room = %d, want 1", n)
			}

			if breakIt == "expel a member" {
				w.clients[w.ids["Leader"]].Send(encodeOustPartyMember("Mate2"))
			} else {
				w.run(t, "Leader", func(_ *script.Script, p *script.Player) { p.TeleportTo(script.Loc{X: -9000, Y: 20, Z: 30}, 0) })
			}
			w.srv.Settle(t)
			w.advance(t, 20*time.Second)

			for _, who := range []string{"Mate1", "Mate2"} {
				if !w.out(t, who) {
					t.Errorf("%s was not put out of the rift", who)
				}
			}
			if breakIt == "expel a member" && !w.out(t, "Leader") {
				t.Error("Leader was not put out of the rift")
			}
			for who := range w.ids {
				if s := w.memo(t, who, 0); s != -1 {
					t.Errorf("%s memo state = %d, want -1", who, s)
				}
			}
			if n := len(w.find(q635Invader)); n != 0 {
				t.Errorf("invaders after the party was put out = %d, want 0", n)
			}

			// The room is free again.
			for who := range w.ids {
				w.back(t, who)
			}
			w.enter(t, "Leader")
			if r := w.roomOf(t, "Leader"); r != 5 {
				t.Errorf("Leader stands in room %d after asking again, want 5", r)
			}
		})
	}
}

// jumpRules are the settings a room's jump follows in these tests: the party
// waits 2 minutes, not the shipped 8, and Anakazel's chance is chance.
func q635JumpRules(chance int) *script.EventRules {
	return q635Rules(func(r *script.EventRules) {
		r.Rift.JumpDelayMin, r.Rift.JumpDelayRnd, r.Rift.AnakazelPortChance = 2, 1, chance
	})
}

// quakes reports whether frames hold the screen shake of a jump.
func quakes(frames [][]byte) bool { return opcodeCount(frames, serverpackets.OpcodeEarthquake) > 0 }

// TestQ635PartyJumpsToAFreeRoom: after the setting's minutes (2 where the
// shipped setting is 8) a room shakes the screens of its party and, five
// seconds on, moves it to the free room the draw picks; the party keeps its
// memo of the time it came, and the room it holds is the new one.
func TestQ635PartyJumpsToAFreeRoom(t *testing.T) {
	t.Parallel()
	w := q635Boot(t, q635Spec{mates: 1, rules: q635JumpRules(10)})
	w.rand.room.Store(5)
	w.rand.anakazel.Store(50)
	w.enter(t, "Leader")
	came := w.memo(t, "Leader", 0)
	w.rand.room.Store(2)

	// 110 seconds and the party stays.
	for range 11 {
		if quakes(w.tick(t)) {
			t.Fatal("the room shook before the setting's minutes")
		}
	}
	if r := w.roomOf(t, "Leader"); r != 5 {
		t.Fatalf("Leader stands in room %d after 110 seconds, want 5", r)
	}
	shaken := false
	for range 3 {
		if quakes(w.tick(t)) {
			shaken = true
			break
		}
	}
	if !shaken {
		t.Fatal("the room never shook")
	}
	w.step(t, 5*time.Second)
	w.arrive(t)
	for who := range w.ids {
		if r := w.roomOf(t, who); r != 2 {
			t.Errorf("%s stands in room %d after the jump, want 2", who, r)
		}
		if s := w.memo(t, who, 0); who == "Leader" && s != came {
			t.Errorf("%s memo state = %d, want the tick it came, %d", who, s, came)
		}
	}

	// The room it holds now is room 2: asking again finds it held.
	for who := range w.ids {
		w.back(t, who)
	}
	got := w.enter(t, "Leader")
	requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_08.htm", "")))
}

// TestQ635PartyJumpsToAnakazel: a jump finds Anakazel's room by the setting's
// chance when the draw falls below it, and not otherwise.
func TestQ635PartyJumpsToAnakazel(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		chance int
		draw   int32
		want   int
	}{
		{"draw below the chance", 10, 5, 8},
		{"draw at the chance", 10, 10, 2},
		{"the shipped chance would not", 3, 5, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, q635Spec{mates: 1, rules: q635JumpRules(c.chance)})
			w.rand.room.Store(5)
			w.enter(t, "Leader")
			w.rand.room.Store(2)
			w.rand.anakazel.Store(c.draw)
			for range 14 {
				w.tick(t)
			}
			if r := w.roomOf(t, "Leader"); r != c.want {
				t.Errorf("Leader stands in room %d after the jump, want %d", r, c.want)
			}
		})
	}
}

// TestQ635JumpAfterFortyMinutesPutsThePartyOut: a party that came forty
// minutes ago, by the time its memo says, is put out of the rift by the
// jump, its members' second memo state set to -1.
func TestQ635JumpAfterFortyMinutesPutsThePartyOut(t *testing.T) {
	t.Parallel()
	w := q635Boot(t, q635Spec{mates: 1, rules: q635JumpRules(10)})
	w.rand.room.Store(5)
	w.enter(t, "Leader")
	w.rand.room.Store(2)
	for who := range w.ids {
		w.run(t, who, func(s *script.Script, p *script.Player) { s.SetMemoState(p, 635, -3000) })
	}
	for range 14 {
		w.tick(t)
	}
	for who := range w.ids {
		if !w.out(t, who) {
			t.Errorf("%s was not put out of the rift once forty minutes were gone", who)
		}
		if s := w.memo(t, who, 1); s != -1 {
			t.Errorf("%s memo state 1 = %d, want -1", who, s)
		}
	}
	if n := len(w.find(q635Invader)); n != 0 {
		t.Errorf("invaders after the party was put out = %d, want 0", n)
	}
}

// TestQ635AnakazelsRoomHoldsOnTheBossEvent: the guardian of Anakazel's room
// keeps its party while the boss has told it 2, and sends it on, shaking
// the screens, once it hears 3.
func TestQ635AnakazelsRoomHoldsOnTheBossEvent(t *testing.T) {
	t.Parallel()
	w := q635Boot(t, q635Spec{mates: 1, rules: q635JumpRules(10)})
	w.rand.room.Store(8)
	w.enter(t, "Leader")
	if r := w.roomOf(t, "Leader"); r != 8 {
		t.Fatalf("Leader stands in room %d, want 8", r)
	}
	w.rand.room.Store(1)
	send := func(event int32) {
		t.Helper()
		g := script.NPCOf(w.guardian(t, 8))
		w.run(t, "Leader", func(s *script.Script, _ *script.Player) { s.SendScriptEvent(g, event, 0, 0) })
	}
	send(2)
	for range 30 {
		w.tick(t)
	}
	if r := w.roomOf(t, "Leader"); r != 8 {
		t.Fatalf("Leader stands in room %d after the boss's 2, want to be kept in 8", r)
	}
	send(3)
	for range 12 {
		w.tick(t)
		if w.roomOf(t, "Leader") != 8 {
			break
		}
	}
	if r := w.roomOf(t, "Leader"); r != 1 {
		t.Errorf("Leader stands in room %d after the boss's 3, want 1", r)
	}
}

// TestQ635GuardianGivesUpAndChangesRoom: a guardian greets with its room's
// page. Its give-up link puts the party out only on the leader's word, a
// change of room moves it to a free room once, on the leader's word, and
// neither answers a character who is not the party's leader.
func TestQ635GuardianGivesUpAndChangesRoom(t *testing.T) {
	t.Parallel()
	for _, link := range []string{"GiveUpRift", "ChangeRiftRoom"} {
		t.Run(link, func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, q635Spec{mates: 1})
			w.rand.room.Store(5)
			w.enter(t, "Leader")
			guardian := w.guardian(t, 5).ObjectID()
			w.roles[guardian] = "guardian"
			page := "tel_dungeon_npc_hi5.htm"
			via := func(who string, events ...string) []string {
				t.Helper()
				return w.talk(t, who, guardian, events...)
			}
			// The greeting itself.
			c := w.clients[w.ids["Leader"]]
			c.Send(encodeTutorialAction(guardian, w.at))
			c.Send(encodeTutorialAction(guardian, w.at))
			requireLine(t, w.lines(t, "Leader"), htmlLine("guardian", q635Page(t, page, "")))

			linkEvent := link
			if link == "GiveUpRift" {
				linkEvent = "tel_dungeon_npc_giveup.htm"
			}
			// A member that is not the leader.
			events := []string{linkEvent}
			if link == "GiveUpRift" {
				events = append(events, link)
			}
			got := via("Mate1", events...)
			requireLine(t, got, htmlLine("guardian", q635Page(t, "tel_dungeon_npc_notleader.htm", "")))
			if r := w.roomOf(t, "Mate1"); r != 5 {
				t.Fatalf("Mate1 stands in room %d, want to be kept in 5", r)
			}

			if link == "GiveUpRift" {
				via("Leader", events...)
				for who := range w.ids {
					if !w.out(t, who) {
						t.Errorf("%s was not put out of the rift", who)
					}
					if s := w.memo(t, who, 1); s != -1 {
						t.Errorf("%s memo state 1 = %d, want -1", who, s)
					}
				}
				return
			}

			w.rand.free.Store(0)
			via("Leader", link)
			w.arrive(t)
			for who := range w.ids {
				if r := w.roomOf(t, who); r != 0 {
					t.Errorf("%s stands in room %d, want the first free room, 0", who, r)
				}
			}
			if s := w.memo(t, "Leader", 1); s != 0 {
				t.Errorf("Leader memo state 1 = %d, want 0", s)
			}
			// No more chances: the guardian of the new room says so.
			next := w.guardian(t, 0).ObjectID()
			w.roles[next] = "guardian"
			got = w.talk(t, "Leader", next, link)
			requireLine(t, got, htmlLine("guardian", q635Page(t, "tel_dungeon_npc_nomorechance.htm", "")))
		})
	}
}

// TestQ635QuakeThenLeaveDoesNothingMore: a party that gives up, or changes
// room, after the room's quake is not touched by the jump five seconds on: it
// stays where it went, and the emptied room can be entered again.
func TestQ635QuakeThenLeaveDoesNothingMore(t *testing.T) {
	t.Parallel()
	for _, link := range []string{"GiveUpRift", "ChangeRiftRoom"} {
		t.Run(link, func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, q635Spec{mates: 1, rules: q635JumpRules(0)})
			w.rand.room.Store(5)
			w.rand.anakazel.Store(50)
			w.enter(t, "Leader")
			guardian := w.guardian(t, 5).ObjectID()
			w.roles[guardian] = "guardian"
			for range 14 {
				if quakes(w.tick(t)) {
					break
				}
			}
			events := []string{link}
			if link == "GiveUpRift" {
				events = []string{"tel_dungeon_npc_giveup.htm", link}
			}
			w.rand.free.Store(0)
			w.talk(t, "Leader", guardian, events...)
			w.arrive(t)
			want := -1
			if link == "ChangeRiftRoom" {
				want = 0
			}
			w.step(t, 5*time.Second)
			w.arrive(t)
			for who := range w.ids {
				if want == -1 && !w.out(t, who) {
					t.Errorf("%s is back in the rift after the jump", who)
				}
				if want != -1 {
					if r := w.roomOf(t, who); r != want {
						t.Errorf("%s stands in room %d after the jump, want %d", who, r, want)
					}
				}
			}
			if want == -1 {
				for who := range w.ids {
					w.back(t, who)
				}
				w.enter(t, "Leader")
				if r := w.roomOf(t, "Leader"); r != 5 {
					t.Errorf("Leader stands in room %d on entering again, want the emptied room 5", r)
				}
			}
		})
	}
}

// TestQ635RiftWatcherSendsHome: the post's way out takes a character to
// where its memo of the way in says, a town by its cabal; its oracle link
// takes a cabal's character to the oracle while the competition lasts.
func TestQ635RiftWatcherSendsHome(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		memo   string
		cabal  sevensigns.Cabal
		wantAt string
	}{
		{"by the keeper", "30000", sevensigns.NoCabal, "45160,123605,-5408"},
		{"town of dawn", "100", sevensigns.Dawn, "-80542,150315,-3040"},
		{"town of dusk", "100", sevensigns.Dusk, "-82340,151575,-3120"},
		{"town without a cabal", "100", sevensigns.NoCabal, ""},
		{"gap between towns", "900", sevensigns.Dawn, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, q635Spec{})
			w.run(t, "Leader", func(_ *script.Script, p *script.Player) { p.SetMemo("rift", c.memo) })
			if c.cabal != sevensigns.NoCabal {
				if err := w.srv.SevenSigns.SetPlayerInfo(context.Background(), w.ids["Leader"], c.cabal, sevensigns.Gnosis); err != nil {
					t.Fatal(err)
				}
			}
			got := w.talk(t, "Leader", w.post.ObjectID(), "rift_watcher_06.htm")
			requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_06.htm", "")))
			if c.wantAt == "" {
				if x, y, _ := w.srv.PlayerPosition(t, w.ids["Leader"]); x != w.at.X || y != w.at.Y {
					t.Errorf("Leader moved to %d,%d", x, y)
				}
				return
			}
			x, y, z := w.srv.PlayerPosition(t, w.ids["Leader"])
			if at := fmt.Sprintf("%d,%d,%d", x, y, z); at != c.wantAt {
				t.Errorf("Leader stands at %s, want %s", at, c.wantAt)
			}
		})
	}

	t.Run("oracle outside the competition", func(t *testing.T) {
		t.Parallel()
		w := q635Boot(t, q635Spec{recruiting: true})
		got := w.talk(t, "Leader", w.post.ObjectID(), "rift_watcher_10.htm")
		requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_12.htm", "")))
	})
	t.Run("oracle without a cabal", func(t *testing.T) {
		t.Parallel()
		w := q635Boot(t, q635Spec{})
		got := w.talk(t, "Leader", w.post.ObjectID(), "rift_watcher_10.htm")
		requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_11.htm", "")))
	})
	for _, cabal := range []sevensigns.Cabal{sevensigns.Dawn, sevensigns.Dusk} {
		t.Run("oracle of "+cabal.String(), func(t *testing.T) {
			t.Parallel()
			w := q635Boot(t, q635Spec{})
			if err := w.srv.SevenSigns.SetPlayerInfo(context.Background(), w.ids["Leader"], cabal, sevensigns.Gnosis); err != nil {
				t.Fatal(err)
			}
			w.talk(t, "Leader", w.post.ObjectID(), "rift_watcher_10.htm")
			x, y, _ := w.srv.PlayerPosition(t, w.ids["Leader"])
			wantY := 86500
			if cabal == sevensigns.Dawn {
				wantY = 111300
			}
			if y < wantY-100 || y > wantY+100 || x > -80000 || x < -82000 {
				t.Errorf("Leader stands at %d,%d, want the oracle of %s", x, y, cabal)
			}
		})
	}
}
