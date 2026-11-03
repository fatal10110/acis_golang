package quest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q620"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// Four Goblets (Q620_FourGoblets) works on a party, a boss zone, doors,
// timers on the clock of the hour, privates and the script events they
// send, none of which a scenario has a line for (#4020 item 5, #4048): the
// sepulcher keepers' Enter, the hall gatekeepers' doors and privates, and
// the ghost of Wigoth a raid boss leaves. These Boot tests play them; the
// scenarios play the dialogs, the goblets and the boxes of one character.
const (
	q620Name  = "Q620_FourGoblets"
	q620Path  = "quest.Q620_FourGoblets"
	q620Probe = "ai.Q620Master"

	q620Keeper     = 31921
	q620Gate       = 31925
	q620GatePlain  = 31926
	q620Wigoth     = 31452
	q620Master     = 31999
	q620Halisha    = 25339
	q620Strong     = 18244
	q620Middle     = 18245
	q620Weak       = 18246
	q620Pass       = 7075
	q620Key        = 7260
	q620UsedPass   = 7261
	q620Brooch     = 7262
	q620Goblet     = 7256
	q620GateDoor   = 25150012
	q620PlainDoor  = 25150013
	q620RoomIndex  = 3
	q620PlainParam = 7
)

// q620Sepulcher is where the Conquerors' keeper takes a party to.
var q620Sepulcher = location.Location{X: 181528, Y: -85583, Z: -7216}

// q620Events logs the script events the master hears.
type q620Events struct {
	mu   sync.Mutex
	list []int32
}

func (e *q620Events) add(id int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, id)
}

func (e *q620Events) take() []int32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.list
	e.list = nil
	return out
}

// q620World is a party in the world beside a sepulcher keeper.
type q620World struct {
	srv     *gameservertest.Server
	ids     map[string]int32
	clients map[int32]*testsupport.ScriptedClient
	at      location.Location
	roles   map[int32]string
	events  *q620Events
	draws   *scriptDraws
	boss    *zone.Boss
}

// q620Spec is how a world differs from the default: a party of four, each
// member with a pass and the quest started, the shipped party size.
type q620Spec struct {
	mates     int
	rules     *script.EventRules
	passless  string // a member without an entrance pass
	questless string // a member that has not started the quest
	brooch    string // a member holding the antique brooch
	past      string // a member past the first step: the quest at its second condition
}

func q620Template(id int, kind string) *npc.Template {
	if kind == "Monster" {
		return proofMonster(id)
	}
	return gameservertest.FolkTemplate(kind, id)
}

func q620Doors() []*door.Template {
	var out []*door.Template
	for i, id := range []int{q620GateDoor, q620PlainDoor} {
		at := location.Location{X: 3000 + i*300, Y: 3000, Z: 30}
		out = append(out, &door.Template{
			ID: id, Name: "hall door", Kind: door.KindDoor, Level: 1, Opened: true, Position: at,
			Coordinates: []location.Point{{X: at.X - 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y + 100}, {X: at.X - 50, Y: at.Y + 100}},
			HP:          100000, PDef: 600, MDef: 500, Height: 1024,
		})
	}
	return out
}

func q620Boot(t *testing.T, spec q620Spec) *q620World {
	t.Helper()
	w := &q620World{events: &q620Events{}, draws: &scriptDraws{}, roles: map[int32]string{}}
	pages := shippedQuestPages(t, q620Name)
	chatPages(pages, q620Keeper, q620Gate, q620GatePlain)

	gate := q620Template(q620Gate, "Folk")
	gate.AIParams = npc.AIParams{
		"SpawnX1": "3100", "SpawnY1": "3100", "SpawnZ1": "30",
		"SpawnX2": "3200", "SpawnY2": "3200", "SpawnZ2": "30",
		"SpawnX3": "3300", "SpawnY3": "3300", "SpawnZ3": "30",
		"RoomIDX": strconv.Itoa(q620RoomIndex),
	}
	templates := []*npc.Template{
		q620Template(q620Keeper, "Folk"), gate, q620Template(q620GatePlain, "Folk"), q620Template(q620Wigoth, "Folk"),
		q620Template(q620Master, "Monster"), q620Template(q620Halisha, "Monster"),
		q620Template(q620Strong, "Monster"), q620Template(q620Middle, "Monster"), q620Template(q620Weak, "Monster"),
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindFolk
		if tmpl.Type == "Monster" {
			kinds[int32(tmpl.ID)] = script.KindHostile
		}
	}
	catalog := script.Catalog{
		q620Path: q620.New,
		q620Probe: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{q620Master}, Hooks: script.Hooks{
				OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) { w.events.add(e.EventID) },
			}}
		},
	}
	form, err := zone.NewCuboid(q620Sepulcher.X-2000, q620Sepulcher.X+2000, q620Sepulcher.Y-2000, q620Sepulcher.Y+2000, -9000, -5000)
	if err != nil {
		t.Fatal(err)
	}
	// The zone polices entry, so a member who walks in without the keeper's
	// grant is ousted.
	boss := zone.NewBoss(620, form)
	boss.InvadeWindow = 10 * time.Minute
	boss.OustLoc = location.Location{X: q620Sepulcher.X + 20000, Y: q620Sepulcher.Y, Z: q620Sepulcher.Z}
	w.boss = boss
	zones := zone.NewIndex()
	zones.Add(boss)

	items := append(q620.New().Items, q620Pass, q620Brooch, 7254, q620Goblet)
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Leader", 75, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q620Path}, {Path: q620Probe}}, catalog),
		gameservertest.WithScriptRand(w.draws.rand),
		gameservertest.WithDoors(q620Doors()...),
		gameservertest.WithZones(zones),
		x1Items(items),
	}
	if spec.rules != nil {
		opts = append(opts, gameservertest.WithEventRules(*spec.rules))
	}
	srv := gameservertest.Boot(t, opts...)
	w.srv = srv

	seed := func(id int32, name string) {
		if name != spec.questless {
			insertJournal(t, srv.DB,
				journalRow{id, q620Name, "<state>", val("STARTED")},
				journalRow{id, q620Name, "<cond>", val(map[bool]string{false: "1", true: "2"}[name == spec.past])},
			)
		}
		if name == "Mate2" {
			// A mark left by an earlier visit.
			insertJournal(t, srv.DB, journalRow{id, q620Name, "completed", val("true")})
		}
		if name != spec.passless {
			srv.GiveItem(t, id, q620Pass, 1)
		}
		if name == spec.brooch {
			srv.GiveItem(t, id, q620Brooch, 1)
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
	return w
}

// give adds count of item id to who's inventory in the world.
func (w *q620World) give(t *testing.T, who string, id, count int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.ids[who], q620Name, func(s *script.Script, p *script.Player) { s.GiveItems(p, id, count) }) {
		t.Fatal("giving the item panicked")
	}
	w.lines(t, who)
}

// toMinute moves the clock the scripts run on to second sec of the next
// minute min of an hour.
func (w *q620World) toMinute(t *testing.T, minute, sec int) {
	t.Helper()
	var now time.Time
	if !w.srv.RunScript(t, w.ids["Leader"], q620Name, func(s *script.Script, _ *script.Player) { now = s.Now() }) {
		t.Fatal("the clock read panicked")
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, sec, 0, now.Location())
	if !at.After(now) {
		at = at.Add(time.Hour)
	}
	w.srv.Advance(t, at.Sub(now))
	for _, c := range w.clients {
		w.srv.ReadQueued(t, c)
	}
}

// folk spawns a civilian of id beside the party and names it for the
// renders.
func (w *q620World) folk(t *testing.T, role string, id int) *npc.Folk {
	t.Helper()
	f := w.srv.SpawnFolkNPCAt(t, q620Template(id, "Folk"), w.at)
	w.roles[f.ObjectID()] = role
	for _, c := range w.clients {
		w.srv.ReadQueued(t, c)
	}
	return f
}

// lines renders what the server queued to who's client.
func (w *q620World) lines(t *testing.T, who string) []string {
	t.Helper()
	var out []string
	for _, f := range w.srv.ReadQueued(t, w.clients[w.ids[who]]) {
		if deferredUpdate(f) {
			continue
		}
		// Packets the goldens do not render (a teleport's stops and moves) are
		// not what these tests look at.
		if line, err := scriptcontract.Packet(f, w.roles); err == nil {
			out = append(out, line)
		}
	}
	return out
}

// talk has who click the NPC twice, open its quest window and follow the
// links of events in turn, and returns what the last answer sent.
func (w *q620World) talk(t *testing.T, who string, obj int32, events ...string) []string {
	t.Helper()
	c := w.clients[w.ids[who]]
	for range 2 {
		c.Send(encodeTutorialAction(obj, w.at))
		w.srv.ReadQueued(t, c)
	}
	c.Send(encodeBypass(fmt.Sprintf("npc_%d_Quest", obj)))
	out := w.lines(t, who)
	for _, ev := range events {
		c.Send(encodeBypass("Quest " + q620Name + " " + ev))
		out = w.lines(t, who)
	}
	return out
}

// arrive has every member take a step where he stands, which is when a
// zone takes in who jumped into it.
func (w *q620World) arrive(t *testing.T) {
	t.Helper()
	w.srv.Advance(t, time.Second)
	for _, id := range w.ids {
		x, y, z := w.srv.PlayerPosition(t, id)
		from := location.Location{X: x, Y: y, Z: z}
		w.clients[id].Send(encodeMoveBackwardToLocation(location.Location{X: x + 30, Y: y, Z: z}, from))
	}
	w.srv.Advance(t, 5*time.Second)
	for _, c := range w.clients {
		w.srv.ReadQueued(t, c)
	}
}

// enter has who ask the keeper obj to let the party in, from its first
// page, and returns the lines it got.
func (w *q620World) enter(t *testing.T, who string, obj int32) []string {
	t.Helper()
	return w.talk(t, who, obj, "Enter")
}

// htmlLine is the NpcHtmlMessage line of the NPC role showing text.
func htmlLine(role, text string) string {
	return "S NpcHtmlMessage obj=" + role + " item=0 html=" + scriptcontract.Quote(text)
}

func keeperPage(t *testing.T, suffix string) string {
	t.Helper()
	return datapackPage(t, "script/quest/"+q620Name+"/"+strconv.Itoa(q620Keeper)+suffix)
}

// requireLine fails unless lines hold want.
func requireLine(t *testing.T, lines []string, want string) {
	t.Helper()
	if !slices.Contains(lines, want) {
		t.Fatalf("lines = %q, want to hold %q", lines, want)
	}
}

// combatants returns the NPCs of id in the world.
func q620Combatants(w *q620World, id int32) []attackable.Combatant {
	var out []attackable.Combatant
	for _, obj := range w.srv.State.Objects() {
		c, ok := obj.(attackable.Combatant)
		if !ok {
			continue
		}
		if n := script.NPCOf(c); n != nil && n.NpcID() == id {
			out = append(out, c)
		}
	}
	return out
}

// TestQ620KeeperTurnsAwayWhoCannotEnter: the keeper answers a party outside
// the last minutes of the hour with its closing-time page, a smaller party
// than the setting asks for, or none, with its party-size page, a request
// from a member who is not the leader with the leader's page, a member
// without a pass or without the quest started with the page naming him.
func TestQ620KeeperTurnsAwayWhoCannotEnter(t *testing.T) {
	t.Parallel()
	five := script.DefaultEventRules()
	five.FourGobletsParty = 5
	cases := []struct {
		name   string
		spec   q620Spec
		minute int
		who    string
		page   string
		member string
	}{
		{"closed", q620Spec{mates: 3}, 10, "Leader", "-02.htm", ""},
		{"too small for the setting", q620Spec{mates: 3, rules: &five}, 56, "Leader", "-04.htm", ""},
		{"no party", q620Spec{}, 56, "Leader", "-04.htm", ""},
		{"not the leader", q620Spec{mates: 3}, 56, "Mate1", "-03.htm", ""},
		{"no pass", q620Spec{mates: 3, passless: "Mate2"}, 56, "Leader", "-05.htm", "Mate2"},
		{"no quest", q620Spec{mates: 3, questless: "Mate3"}, 56, "Leader", "-06.htm", "Mate3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := q620Boot(t, c.spec)
			keeper := w.folk(t, "keeper", q620Keeper)
			w.toMinute(t, c.minute, 10)
			got := w.enter(t, c.who, keeper.ObjectID())
			text := strings.ReplaceAll(keeperPage(t, c.page), "%member%", c.member)
			requireLine(t, got, htmlLine("keeper", text))
			for name, id := range w.ids {
				if n := w.srv.PlayerItemCount(t, id, q620Pass); n != 1 && name != c.spec.passless {
					t.Errorf("%s holds %d passes, want the one he had", name, n)
				}
			}
			if x, y, _ := w.srv.PlayerPosition(t, w.ids["Leader"]); x != w.at.X || y != w.at.Y {
				t.Errorf("Leader moved to %d,%d", x, y)
			}
		})
	}
}

// TestQ620KeeperTakesOnePartyAtATime: a party of four in the last minutes
// of the hour, each holding a pass and the started quest, is taken to the
// sepulcher: Leader's chapel key and every pass are taken, each member but
// the one holding the antique brooch gets a used grave pass, the quest's
// completed mark is cleared, and the zone lets them stay. Another request
// finds the sepulcher taken, and one more finds it free once the keeper's
// slot is cleared.
func TestQ620KeeperTakesOnePartyAtATime(t *testing.T) {
	t.Parallel()
	w := q620Boot(t, q620Spec{mates: 3, brooch: "Mate1"})
	keeper := w.folk(t, "keeper", q620Keeper)
	w.give(t, "Leader", q620Key, 1)
	w.toMinute(t, 56, 10)

	got := w.enter(t, "Leader", keeper.ObjectID())
	requireLine(t, got, htmlLine("keeper", keeperPage(t, "-08.htm")))

	w.arrive(t)
	for name, id := range w.ids {
		x, y, _ := w.srv.PlayerPosition(t, id)
		if dx, dy := x-q620Sepulcher.X, y-q620Sepulcher.Y; dx < -80 || dx > 80 || dy < -80 || dy > 80 {
			t.Errorf("%s stands at %d,%d, want within 80 of the sepulcher", name, x, y)
		}
		wantUsed := 1
		if name == "Mate1" {
			wantUsed = 0
		}
		if n := w.srv.PlayerItemCount(t, id, q620Pass); n != 0 {
			t.Errorf("%s holds %d passes, want 0", name, n)
		}
		if n := w.srv.PlayerItemCount(t, id, q620UsedPass); n != wantUsed {
			t.Errorf("%s holds %d used grave passes, want %d", name, n, wantUsed)
		}
		if v := w.srv.QuestVars(t, id, q620Name)["completed"]; v != "false" {
			t.Errorf("%s's completed = %q, want false", name, v)
		}
	}
	if n := w.srv.PlayerItemCount(t, w.ids["Leader"], q620Key); n != 0 {
		t.Errorf("Leader holds %d chapel keys, want 0", n)
	}

	// The party stands in front of the keeper again, with new passes.
	for name := range w.ids {
		w.give(t, name, q620Pass, 1)
	}
	w.srv.RunScript(t, w.ids["Leader"], q620Name, func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: w.at.X, Y: w.at.Y, Z: w.at.Z}, 0)
	})
	w.srv.Advance(t, time.Second)
	w.clients[w.ids["Leader"]].Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	w.srv.Settle(t)
	w.lines(t, "Leader")
	got = w.enter(t, "Leader", keeper.ObjectID())
	requireLine(t, got, htmlLine("keeper", keeperPage(t, "-07.htm")))
	if n := w.srv.PlayerItemCount(t, w.ids["Leader"], q620Pass); n != 1 {
		t.Errorf("Leader holds %d passes after the refusal, want 1", n)
	}

	w.srv.RunScript(t, w.ids["Leader"], q620Name, func(_ *script.Script, _ *script.Player) {
		for _, n := range npcsOf(w.srv, q620Keeper) {
			n.SetInt(script.SlotAVQuest0, 0)
		}
	})
	got = w.enter(t, "Leader", keeper.ObjectID())
	requireLine(t, got, htmlLine("keeper", keeperPage(t, "-08.htm")))
}

// TestQ620KeeperLeavesBehindWhoStandsFarOff: a member more than 1000 from the
// keeper when Leader asks to enter is not taken: he keeps his pass, gets no
// used grave pass, stays where he stands with his completed mark, while the
// rest of the party goes in.
func TestQ620KeeperLeavesBehindWhoStandsFarOff(t *testing.T) {
	t.Parallel()
	w := q620Boot(t, q620Spec{mates: 3})
	keeper := w.folk(t, "keeper", q620Keeper)
	w.toMinute(t, 56, 10)

	far := location.Location{X: w.at.X, Y: w.at.Y + 1500, Z: w.at.Z}
	w.srv.RunScript(t, w.ids["Mate2"], q620Name, func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: far.X, Y: far.Y, Z: far.Z}, 0)
	})
	w.srv.Advance(t, time.Second)
	w.clients[w.ids["Mate2"]].Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	w.srv.Settle(t)
	if x, y, _ := w.srv.PlayerPosition(t, w.ids["Mate2"]); x != far.X || y != far.Y {
		t.Fatalf("Mate2 stands at %d,%d, want the far spot %d,%d", x, y, far.X, far.Y)
	}

	got := w.enter(t, "Leader", keeper.ObjectID())
	requireLine(t, got, htmlLine("keeper", keeperPage(t, "-08.htm")))
	w.arrive(t)

	id := w.ids["Mate2"]
	// The step every member takes on arrival moves him 30 at most.
	if x, y, _ := w.srv.PlayerPosition(t, id); x < far.X || x > far.X+30 || y != far.Y {
		t.Errorf("Mate2 moved to %d,%d, want to stay at %d,%d", x, y, far.X, far.Y)
	}
	if n := w.srv.PlayerItemCount(t, id, q620Pass); n != 1 {
		t.Errorf("Mate2 holds %d passes, want the one he had", n)
	}
	if n := w.srv.PlayerItemCount(t, id, q620UsedPass); n != 0 {
		t.Errorf("Mate2 holds %d used grave passes, want 0", n)
	}
	if v := w.srv.QuestVars(t, id, q620Name)["completed"]; v != "true" {
		t.Errorf("Mate2's completed = %q, want true", v)
	}
	for _, name := range []string{"Leader", "Mate1", "Mate3"} {
		x, y, _ := w.srv.PlayerPosition(t, w.ids[name])
		if dx, dy := x-q620Sepulcher.X, y-q620Sepulcher.Y; dx < -80 || dx > 80 || dy < -80 || dy > 80 {
			t.Errorf("%s stands at %d,%d, want within 80 of the sepulcher", name, x, y)
		}
		if n := w.srv.PlayerItemCount(t, w.ids[name], q620UsedPass); n != 1 {
			t.Errorf("%s holds %d used grave passes, want 1", name, n)
		}
	}
}

// TestQ620GatekeeperSpotOrder: the draw of the gatekeeper's three spots
// orders the strong, middle and weak privates: 0 takes the spots in order,
// with the weak one at the third spot's x and the second spot's y, 1 takes
// the second, third and first, 2 the third, first and second.
func TestQ620GatekeeperSpotOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		draw int
		want [3]location.Location // strong, middle, weak
	}{
		{0, [3]location.Location{{X: 3100, Y: 3100}, {X: 3200, Y: 3200}, {X: 3300, Y: 3200}}},
		{1, [3]location.Location{{X: 3200, Y: 3200}, {X: 3300, Y: 3300}, {X: 3100, Y: 3100}}},
		{2, [3]location.Location{{X: 3300, Y: 3300}, {X: 3100, Y: 3100}, {X: 3200, Y: 3200}}},
	}
	for _, c := range cases {
		t.Run(strconv.Itoa(c.draw), func(t *testing.T) {
			t.Parallel()
			w := q620Boot(t, q620Spec{})
			w.srv.SpawnHostileNPCTemplateAt(t, q620Template(q620Master, "Monster"), w.at)
			w.toMinute(t, 20, 10)
			gate := w.spawnGate(t, "gate", q620Gate, 0)
			w.give(t, "Leader", q620Key, 1)
			w.draws.queue(c.draw)
			w.talk(t, "Leader", gate.ObjectID())
			for i, id := range []int32{q620Strong, q620Middle, q620Weak} {
				list := q620Combatants(w, id)
				if len(list) != 1 {
					t.Fatalf("privates %d = %d, want 1", id, len(list))
				}
				if x, y, _ := list[0].Position(); x != c.want[i].X || y != c.want[i].Y {
					t.Errorf("private %d stands at %d,%d, want %d,%d", id, x, y, c.want[i].X, c.want[i].Y)
				}
			}
		})
	}
}

// spawnGate puts a hall gatekeeper of npcID under the master as one of its
// privates, with room as its first spawn parameter, beside the party.
func (w *q620World) spawnGate(t *testing.T, role string, npcID int32, room int32) *script.NPC {
	t.Helper()
	var gate *script.NPC
	w.srv.RunScript(t, w.ids["Leader"], q620Name, func(s *script.Script, _ *script.Player) {
		masters := npcsOf(w.srv, q620Master)
		if len(masters) != 1 {
			t.Errorf("masters = %d, want 1", len(masters))
			return
		}
		gate = s.Spawner().CreateOnePrivateEx(masters[0], npcID, script.Loc{X: w.at.X, Y: w.at.Y, Z: w.at.Z}, 0, script.Params{P1: room})
	})
	if gate == nil {
		t.Fatal("no gatekeeper")
	}
	w.roles[gate.ObjectID()] = role
	w.srv.Advance(t, time.Second)
	for _, c := range w.clients {
		w.srv.ReadQueued(t, c)
	}
	return gate
}

func (w *q620World) doorOpen(t *testing.T, id int) bool {
	t.Helper()
	_, open := w.srv.DoorState(t, id)
	return open
}

// TestQ620GatekeeperOpensItsDoorForAKey: a hall gatekeeper closes its door
// when it appears. Leader's chapel key opens it for 15 seconds and calls
// three privates, strong, middle and weak, at the three spawn spots in the
// order the draw picks, each with its own spawn parameter, and tells the
// master of its room by script event; the gatekeeper shouts. A gatekeeper
// with no spots only tells the master of the room its own parameter names.
// Without a key it opens nothing.
func TestQ620GatekeeperOpensItsDoorForAKey(t *testing.T) {
	t.Parallel()
	w := q620Boot(t, q620Spec{})
	w.srv.SpawnHostileNPCTemplateAt(t, q620Template(q620Master, "Monster"), w.at)
	w.toMinute(t, 20, 10)

	gate := w.spawnGate(t, "gate", q620Gate, 0)
	plain := w.spawnGate(t, "plain", q620GatePlain, q620PlainParam)
	if w.doorOpen(t, q620GateDoor) || w.doorOpen(t, q620PlainDoor) {
		t.Fatal("a hall door stands open after its gatekeeper appeared")
	}

	// No key: the page of a gatekeeper, no door.
	got := w.talk(t, "Leader", gate.ObjectID())
	requireLine(t, got, htmlLine("gate", datapackPage(t, "script/quest/"+q620Name+"/31925-01.htm")))
	if w.doorOpen(t, q620GateDoor) {
		t.Fatal("the door opened without a key")
	}

	w.give(t, "Leader", q620Key, 2)
	w.draws.queue(1)
	got = w.talk(t, "Leader", gate.ObjectID())
	if n := w.srv.PlayerItemCount(t, w.ids["Leader"], q620Key); n != 1 {
		t.Errorf("chapel keys after the first opening = %d, want 1", n)
	}
	if !w.doorOpen(t, q620GateDoor) {
		t.Fatal("the door stays shut with a key")
	}
	requireLine(t, got, `S NpcSay obj=gate type=1 npc=1031925 text="The monsters have spawned!"`)
	if events := w.events.take(); !slices.Equal(events, []int32{1000 + q620RoomIndex}) {
		t.Errorf("master events = %v, want [%d]", events, 1000+q620RoomIndex)
	}
	// A draw of 1 picks the second, third and first spots.
	want := map[int32]location.Location{
		q620Strong: {X: 3200, Y: 3200}, q620Middle: {X: 3300, Y: 3300}, q620Weak: {X: 3100, Y: 3100},
	}
	for id, at := range want {
		list := q620Combatants(w, id)
		if len(list) != 1 {
			t.Errorf("privates %d = %d, want 1", id, len(list))
			continue
		}
		if x, y, _ := list[0].Position(); x != at.X || y != at.Y {
			t.Errorf("private %d stands at %d,%d, want %d,%d", id, x, y, at.X, at.Y)
		}
		params := map[int32]int32{q620Strong: 1, q620Middle: 2, q620Weak: 3}
		if got := npcsOf(w.srv, id)[0].Int(script.SlotParam1); got != params[id] {
			t.Errorf("private %d's first parameter = %d, want %d", id, got, params[id])
		}
	}

	// The door closes 15 seconds after the opening.
	w.srv.Advance(t, 14*time.Second)
	if !w.doorOpen(t, q620GateDoor) {
		t.Fatal("the door closed early")
	}
	w.srv.Advance(t, 2*time.Second)
	if w.doorOpen(t, q620GateDoor) {
		t.Fatal("the door stays open after 15 seconds")
	}

	// A gatekeeper with no spots calls no privates, and tells the room its
	// parameter names.
	w.talk(t, "Leader", plain.ObjectID())
	if !w.doorOpen(t, q620PlainDoor) {
		t.Fatal("the plain gatekeeper's door stays shut with a key")
	}
	if events := w.events.take(); !slices.Equal(events, []int32{1000 + q620PlainParam}) {
		t.Errorf("master events = %v, want [%d]", events, 1000+q620PlainParam)
	}
	if n := len(q620Combatants(w, q620Strong)); n != 1 {
		t.Errorf("privates after the plain opening = %d, want the first one only", n)
	}
}

// TestQ620GatekeeperDoorsCloseAtTheHourMark: a door a key opened at 54:50
// is shut by the gatekeeper's own watch at 55:00, not by the 15 seconds,
// which would end at 55:05.
func TestQ620GatekeeperDoorsCloseAtTheHourMark(t *testing.T) {
	t.Parallel()
	w := q620Boot(t, q620Spec{})
	w.srv.SpawnHostileNPCTemplateAt(t, q620Template(q620Master, "Monster"), w.at)
	w.toMinute(t, 54, 40)
	gate := w.spawnGate(t, "plain", q620GatePlain, q620PlainParam)
	w.toMinute(t, 54, 50)
	w.give(t, "Leader", q620Key, 1)
	w.talk(t, "Leader", gate.ObjectID())
	if !w.doorOpen(t, q620PlainDoor) {
		t.Fatal("the door stays shut with a key")
	}
	w.toMinute(t, 54, 59)
	if !w.doorOpen(t, q620PlainDoor) {
		t.Fatal("the door closed before the hour mark")
	}
	w.srv.Advance(t, 2*time.Second)
	if w.doorOpen(t, q620PlainDoor) {
		t.Fatal("the door stays open after the hour mark")
	}
}

// TestQ620HalishaLeavesTheGhostOfWigoth: the party's killing of a raid boss
// pays its goblet to each member on the quest who has none and is not past
// the first step, marks them all as back, and leaves the ghost of Wigoth
// where the boss fell. The ghost leaves with the sepulchers' closing, at 54
// minutes past the hour.
func TestQ620HalishaLeavesTheGhostOfWigoth(t *testing.T) {
	t.Parallel()
	w := q620Boot(t, q620Spec{mates: 2, past: "Mate1"})
	w.give(t, "Mate2", q620Goblet, 1)
	w.toMinute(t, 20, 10)

	x, y, z := w.srv.PlayerPosition(t, w.ids["Leader"])
	boss := w.srv.SpawnHostileNPCTemplateAt(t, q620Template(q620Halisha, "Monster"), location.Location{X: x + 40, Y: y, Z: z})
	killOnPlayerQueue(t, w.srv, w.ids["Leader"], boss)

	for name, goblets := range map[string]int{"Leader": 1, "Mate1": 0, "Mate2": 1} {
		if n := w.srv.PlayerItemCount(t, w.ids[name], q620Goblet); n != goblets {
			t.Errorf("%s holds %d goblets, want %d", name, n, goblets)
		}
		if v := w.srv.QuestVars(t, w.ids[name], q620Name)["completed"]; v != "true" {
			t.Errorf("%s's completed = %q, want true", name, v)
		}
	}
	if n := len(npcsOf(w.srv, q620Wigoth)); n != 1 {
		t.Fatalf("ghosts of Wigoth = %d, want 1", n)
	}
	got := q620Combatants(w, q620Wigoth)[0]
	if gx, gy, _ := got.Position(); gx != x+40 || gy != y {
		t.Errorf("the ghost stands at %d,%d, want where the boss fell, %d,%d", gx, gy, x+40, y)
	}

	// The watch deletes the ghost once the hour reaches 54 minutes.
	w.toMinute(t, 53, 59)
	if n := len(npcsOf(w.srv, q620Wigoth)); n != 1 {
		t.Fatalf("ghosts of Wigoth before 54 minutes = %d, want 1", n)
	}
	w.srv.Advance(t, 3*time.Second)
	if n := len(npcsOf(w.srv, q620Wigoth)); n != 0 {
		t.Fatalf("ghosts of Wigoth after 54 minutes = %d, want 0", n)
	}
}
