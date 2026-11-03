package party

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The Frintezza teleporter (#198 PR 4) at the tomb's entrance, with three
// listener NPCs standing in for Frintezza (global memo key 4) and his
// follower's two forms (keys 5 and 6): each claims its key as it appears,
// keeps a spawn_data row whose db value a test sets, and records the script
// events it hears.
const (
	fztTeleporterID = int32(32011)
	fztMemo4ID      = int32(29995)
	fztMemo5ID      = int32(29996)
	fztMemo6ID      = int32(29997)
	fztScrollID     = int32(8073)
	fztArrowID      = int32(8192)
	fztDewdropID    = int32(8556)
	fztTombZoneID   = 110012
	fztOverweight   = 1118
	fztDirector     = "FztDirector"
	fztRowMemo4     = "fzt_memo4"
)

// fztEntrance is where the characters enter the world and where the
// teleporter sends the players of its territory back to.
var fztEntrance = location.Location{X: 150037, Y: -57255, Z: -2976}

// fztTombSpots are the five landing spots, one per party of the channel.
var fztTombSpots = []location.Location{
	{X: 173247, Y: -76979, Z: -5104},
	{X: 174985, Y: -75161, Z: -5104},
	{X: 173917, Y: -76109, Z: -5104},
	{X: 174037, Y: -76321, Z: -5104},
	{X: 173249, Y: -75154, Z: -5104},
}

// fztMakers are the tomb's npcmakers the teleporter signals: the patrol,
// wizards 1 to 5 and the four alarms.
var fztMakers = []string{
	"godard32_2515_05m1", "godard32_2515_01m1", "godard32_2515_02m1", "godard32_2515_03m1", "godard32_2515_04m1",
	"godard32_2515_08m1", "godard32_2515_22m1", "godard32_2515_19m1", "godard32_2515_20m1", "godard32_2515_21m1",
}

// fztDoorNames are the tomb's doors in the order the teleporter sets them.
var fztDoorNames = []string{
	"grave_pathway_1", "grave_pathway_2", "grave_pathway_3", "grave_pathway_4",
	"wall_door_a_center_1", "wall_door_a_center_2", "wall_door_a_center_3", "wall_door_a_center_4",
	"wall_door_a_center_5", "wall_door_a_center_6", "wall_door_a_center_7", "wall_door_a_center_8",
	"wall_door_b_left_1", "wall_door_b_left_2", "wall_door_b_left_3", "wall_door_b_left_4", "wall_door_b_left_5",
	"wall_door_b_right_1", "wall_door_b_right_2", "wall_door_b_right_3", "wall_door_b_right_4", "wall_door_b_right_5",
}

func fztDoorID(i int) int { return 25150001 + i }

// fztSpawnlist places the listeners, each under its own database maker,
// then the teleporter in a territory holding the entrance and the tomb,
// and declares the tomb's makers as probes.
func fztSpawnlist() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="fzt_memo" minZ="-3100" maxZ="-2900"><node x="149500" y="-58500"/><node x="149700" y="-58500"/><node x="149700" y="-58300"/><node x="149500" y="-58300"/></territory>
	<territory name="fzt_gate" minZ="-6000" maxZ="0"><node x="149000" y="-78000"/><node x="176000" y="-78000"/><node x="176000" y="-56000"/><node x="149000" y="-56000"/></territory>
`)
	for i, id := range []int32{fztMemo4ID, fztMemo5ID, fztMemo6ID} {
		fmt.Fprintf(&b, `	<npcmaker name="fzt_memo%d" territory="fzt_memo" maximumNpcs="1"><ai type="default_use_db_maker"/><npc id="%d" pos="%d;-58400;-2976;0" total="1" respawn="1min" dbName="fzt_memo%d"/></npcmaker>
`, i+4, id, 149550+i*50, i+4)
	}
	b.WriteString(`	<npcmaker name="fzt_teleporter" territory="fzt_gate" maximumNpcs="1"><npc id="32011" pos="150137;-57255;-2976;0" total="1" respawn="1min"/></npcmaker>
`)
	for _, name := range fztMakers {
		fmt.Fprintf(&b, `	<npcmaker name="%s" territory="fzt_memo" maximumNpcs="1"><ai type="fzt_probe"/></npcmaker>
`, name)
	}
	b.WriteString(`</list>`)
	return b.String()
}

// fztPages are the teleporter's pages, each naming itself, the first with
// the shipped page's links.
func fztPages() map[string]string {
	pages := map[string]string{}
	for _, name := range []string{"001a", "002", "003", "004", "005", "006", "007", "008"} {
		page := "frintessa_teleporter" + name + ".htm"
		pages["script/ai/boss/frintezza/FrintezzaTeleporter/"+page] = "<html><body>" + page + "</body></html>"
	}
	pages["script/ai/boss/frintezza/FrintezzaTeleporter/frintessa_teleporter001.htm"] = `<html><body>frintessa_teleporter001.htm<a action="bypass -h Quest FrintezzaTeleporter story">Story</a><a action="bypass -h Quest FrintezzaTeleporter enter">Enter</a></body></html>`
	return pages
}

// fztLog records what the listeners hear and the probe makers run.
type fztLog struct {
	mu     sync.Mutex
	heard  []string
	makers []string
}

func (l *fztLog) add(to *[]string, s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*to = append(*to, s)
}

// take returns and forgets what the listeners heard and the makers ran.
func (l *fztLog) take() (heard, makers []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	heard, makers = l.heard, l.makers
	l.heard, l.makers = nil, nil
	return heard, makers
}

// fztWorld is a group at the tomb's entrance with the teleporter.
type fztWorld struct {
	*group
	log  *fztLog
	tomb *zone.Boss
}

// fztSetup is a teleporter boot: the seats (the first Leader, the leader of
// a level 5 clan holding two Strategy Guides as it enters the world), Frintezza's saved db value
// (none for a fresh row), and extra options.
type fztSetup struct {
	seats  int
	memo4  *int
	extras []gameservertest.Option
}

// bootTeleporter boots the setup, every character entering the world at
// the entrance beside the teleporter.
func bootTeleporter(t *testing.T, s fztSetup) *fztWorld {
	t.Helper()
	if s.memo4 != nil {
		row := &spawn.State{Name: fztRowMemo4, Status: spawn.StatusAlive, CurrentHP: 1 << 20, CurrentMP: 1 << 20, Location: location.Location{X: 149550, Y: -58400, Z: -2976}, DBValue: *s.memo4}
		if err := gamesql.NewSpawnStore(sqltest.SharedDB(t)).SaveStates(context.Background(), map[string]*spawn.State{fztRowMemo4: row}); err != nil {
			t.Fatalf("seed spawn data: %v", err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fzt.xml"), []byte(fztSpawnlist()), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	log := &fztLog{}
	keys := map[int32]string{fztMemo4ID: "4", fztMemo5ID: "5", fztMemo6ID: "6"}
	const memo = "test.FztMemo"
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":              defaultnpc.New,
		"script.ai.boss.frintezza.FrintezzaTeleporter": frintezza.FrintezzaTeleporter,
		"test." + fztDirector:                          func() script.Script { return script.Script{} },
		memo: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{fztMemo4ID, fztMemo5ID, fztMemo6ID}, Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) {
					s.GlobalMemo().Set(keys[e.NPC.NpcID()], int(e.NPC.ObjectID()))
				},
				OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
					log.add(&log.heard, fmt.Sprintf("%s event=%d arg1=%d arg2=%d", keys[e.NPC.NpcID()], e.EventID, e.Arg1, e.Arg2))
				},
			}}
		},
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: memo},
		{Path: "script.ai.boss.frintezza.FrintezzaTeleporter"},
		{Path: "test." + fztDirector},
	}
	kinds := map[int32]script.NPCKind{}
	var templates []*npc.Template
	for _, id := range []int32{fztMemo4ID, fztMemo5ID, fztMemo6ID, fztTeleporterID} {
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
		kinds[id] = script.KindFolk
	}
	makers := script.MakerCatalog{"fzt_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				log.add(&log.makers, e.Group.Maker().Name+" "+e.Name)
			},
		}}
	}}
	stack := func(id int32, name string) *item.Template {
		return &item.Template{ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{}}
	}
	items := item.NewTable(append(gameservertest.ItemTemplates().All(),
		stack(fztScrollID, "Frintezza's Magic Force Field Removal Scroll"), stack(fztArrowID, "Breaking Arrow"),
		stack(fztDewdropID, "Dewdrop of Destruction"), &item.Template{
			ID: strategyGuideID, Name: "Strategy Guide", Kind: item.KindEtcItem, Duration: -1,
			Destroyable: true, EtcItem: &item.EtcItemDetail{},
		}))
	var doors []*door.Template
	for i, name := range fztDoorNames {
		at := location.Location{X: 3000 + i*300, Y: 3000, Z: 30}
		doors = append(doors, &door.Template{
			ID: fztDoorID(i), Name: name, Kind: door.KindDoor, Level: 1, Opened: true, Position: at,
			Coordinates: []location.Point{{X: at.X - 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y + 100}, {X: at.X - 50, Y: at.Y + 100}},
			HP:          100000, PDef: 600, MDef: 500, Height: 1024,
		})
	}
	tombForm, err := zone.NewCuboid(-100100, -99900, -100, 100, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	tomb := zone.NewBoss(fztTombZoneID, tombForm)
	zones := zone.NewIndex()
	zones.Add(tomb)
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{fztEntrance}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Leader", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithItemTemplates(items),
		authorityClan{level: 5, leads: true}.option(t),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(makers),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithHTMLPages(fztPages()),
		gameservertest.WithDoors(doors...),
		gameservertest.WithZones(zones),
		// The clock stands still between two links of a flow.
		gameservertest.WithReuseDelays(3*time.Second, 0),
	}, s.extras...)...)
	leaderID := srv.SoleObjectID(t)
	// One guide pays for the channel; holding another lets Leader invite
	// more parties into it.
	for range 2 {
		srv.GiveItem(t, leaderID, strategyGuideID, 1)
	}
	g := &group{srv: srv, players: []player{{c: srv.Client, id: leaderID, name: "Leader"}}}
	for i := 1; i < s.seats; i++ {
		account := "member" + string(rune('a'+i-1))
		name := fmt.Sprintf("Seat%02d", i)
		id := srv.SeedCharacterFor(t, account, name, 40, 0).ID
		g.players = append(g.players, player{c: srv.DialClient(t, account, 1), id: id, name: name})
	}
	for _, p := range g.players {
		startInWorld(t, p.c)
	}
	g.quiet(t)
	log.take()
	return &fztWorld{group: g, log: log, tomb: tomb}
}

// teleporter returns the teleporter in the world.
func (w *fztWorld) teleporter(t *testing.T) *npc.Folk {
	t.Helper()
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && int32(f.Instance.Template.ID) == fztTeleporterID && !f.Decayed() {
			out = append(out, f)
		}
	}
	if len(out) != 1 {
		t.Fatalf("teleporters in the world = %d, want 1", len(out))
	}
	return out[0]
}

// talk has seat i click the teleporter until it talks, and returns the
// pages it showed.
func (w *fztWorld) talk(t *testing.T, i int) []string {
	t.Helper()
	p := w.players[i]
	f := w.teleporter(t)
	x, y, z := w.srv.PlayerPosition(t, p.id)
	for range 2 {
		p.c.Send(encodeAction(f.ObjectID(), x, y, z))
		w.srv.Settle(t)
		if pages := fztPageNames(drainFrames(t, p.c)); len(pages) > 0 {
			return pages
		}
	}
	t.Fatalf("%s: talking to the teleporter opened no page", p.name)
	return nil
}

// bypass has seat i talk to the teleporter and follow its link to event,
// and returns every client's frames after the answer.
func (w *fztWorld) bypass(t *testing.T, i int, event string) [][][]byte {
	t.Helper()
	if got := w.talk(t, i); !slices.Equal(got, []string{"frintessa_teleporter001.htm"}) {
		t.Fatalf("%s: first talk pages = %q, want frintessa_teleporter001.htm", w.players[i].name, got)
	}
	cmd := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	cmd.WriteString("Quest FrintezzaTeleporter " + event)
	w.players[i].c.Send(cmd.Bytes())
	w.srv.Settle(t)
	out := make([][][]byte, len(w.players))
	for j, p := range w.players {
		out[j] = drainFrames(t, p.c)
	}
	return out
}

// enterPage has seat i ask to enter, and fails unless it alone is answered,
// with page.
func (w *fztWorld) enterPage(t *testing.T, i int, page string) {
	t.Helper()
	frames := w.bypass(t, i, "enter")
	for j, f := range frames {
		if j == i {
			if got := fztPageNames(f); !slices.Equal(got, []string{page}) {
				t.Fatalf("%s: enter pages = %q (%x), want %s", w.players[i].name, got, opcodes(screenFrames(f)), page)
			}
			continue
		}
		for _, frame := range f {
			switch {
			case frame[0] == serverpackets.OpcodeNpcHtmlMessage, frame[0] == serverpackets.OpcodeSystemMessage,
				frame[0] == serverpackets.OpcodeTeleportToLocation, len(fztScreens([][]byte{frame})) != 0:
				t.Fatalf("%s: %#x after %s's refused entry, want only what it sees of the click", w.players[j].name, frame[0], w.players[i].name)
			}
		}
	}
}

// fztPageNames returns the page each NpcHtmlMessage among frames shows.
func fztPageNames(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNpcHtmlMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32()
		html := strings.TrimPrefix(r.ReadString(), "<html><body>")
		name, _, _ := strings.Cut(html, "<")
		out = append(out, name)
	}
	return out
}

// direct runs fn as the Director script for seat i's player.
func (w *fztWorld) direct(t *testing.T, i int, fn func(s *script.Script, p *script.Player)) {
	t.Helper()
	if !w.srv.RunScript(t, w.players[i].id, fztDirector, fn) {
		t.Fatal("the director panicked")
	}
}

// give hands seat i count of item id, as a script does.
func (w *fztWorld) give(t *testing.T, i int, id, count int32) {
	t.Helper()
	w.direct(t, i, func(s *script.Script, p *script.Player) { s.GiveItems(p, id, count) })
	w.srv.Settle(t)
}

// setMemo4 stores v as the db value of the NPC global memo key 4 names.
func (w *fztWorld) setMemo4(t *testing.T, v int) {
	t.Helper()
	w.direct(t, 0, func(s *script.Script, _ *script.Player) {
		s.GlobalMemo().Creature("4").(*script.NPC).SetDBValue(v)
	})
}

// memoID returns the object id global memo key names.
func (w *fztWorld) memoID(t *testing.T, key string) int32 {
	t.Helper()
	var id int32
	w.direct(t, 0, func(s *script.Script, _ *script.Player) { id = s.GlobalMemo().Creature(key).ObjectID() })
	return id
}

// moveTo teleports seat i to at and has its client appear there, leaving
// every client drained.
func (w *fztWorld) moveTo(t *testing.T, i int, at location.Location) {
	t.Helper()
	w.direct(t, i, func(_ *script.Script, p *script.Player) { p.TeleportTo(script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0) })
	w.srv.Settle(t)
	w.appear(t, i)
	w.quiet(t)
}

// appear has seat i's client appear where its teleport sent it, as a real
// client does, and drains it.
func (w *fztWorld) appear(t *testing.T, i int) {
	t.Helper()
	w.players[i].c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	w.srv.Settle(t)
	drainFrames(t, w.players[i].c)
}

// formChannel invites seat 2k+1 into seat 2k's party for each of the
// parties, then has Leader invite every other party into its channel.
func (w *fztWorld) formChannel(t *testing.T, parties int) {
	t.Helper()
	for k := range parties {
		w.invite(t, 2*k, 2*k+1, 0)
	}
	for k := 1; k < parties; k++ {
		w.askAndAccept(t, 0, 2*k)
	}
	w.quiet(t)
}

// fztScreens returns the ExShowScreenMessage frames among frames.
func fztScreens(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeExtended && binary.LittleEndian.Uint16(f[1:3]) == 0x38 {
			out = append(out, f)
		}
	}
	return out
}

// fztRemaining is the countdown's text for minutes.
func fztRemaining(minutes int) string { return fmt.Sprintf("%d minute(s) are remaining.", minutes) }

// wantDoors fails unless the tomb's doors stand open as open says.
func (w *fztWorld) wantDoors(t *testing.T, when string, open func(i int) bool) {
	t.Helper()
	for i, name := range fztDoorNames {
		if _, got := w.srv.DoorState(t, fztDoorID(i)); got != open(i) {
			t.Fatalf("%s: door %s open = %v, want %v", when, name, got, open(i))
		}
	}
}

// TestFrintezzaTeleporterPages: the first talk, the story, the overweight
// refusal (a system message alone), the memo checks and the party check of
// a fresh teleporter; and a teleporter created beside a Frintezza already
// at stage 3, which copies that stage, refuses entry with 007 (002 while his
// follower's transform is unnamed), heeds Frintezza's script events 0 and
// 8 alone, and every minute sends the players of its territory out while
// Frintezza stays at 3 or over, clearing its stage once Frintezza and his
// follower are both back at 0.
func TestFrintezzaTeleporterPages(t *testing.T) {
	t.Parallel()
	t.Run("fresh", func(t *testing.T) {
		t.Parallel()
		w := bootTeleporter(t, fztSetup{seats: 1})
		frames := w.bypass(t, 0, "story")
		if got := fztPageNames(frames[0]); !slices.Equal(got, []string{"frintessa_teleporter001a.htm"}) {
			t.Fatalf("story pages = %q, want frintessa_teleporter001a.htm", got)
		}
		six := w.memoID(t, "6")
		w.direct(t, 0, func(s *script.Script, _ *script.Player) { s.GlobalMemo().Remove("6") })
		w.enterPage(t, 0, "frintessa_teleporter002.htm")
		w.direct(t, 0, func(s *script.Script, _ *script.Player) { s.GlobalMemo().Set("6", int(six)) })
		w.enterPage(t, 0, "frintessa_teleporter003.htm")
	})
	t.Run("overweight", func(t *testing.T) {
		t.Parallel()
		w := bootTeleporter(t, fztSetup{seats: 1, extras: []gameservertest.Option{gameservertest.WithInventorySlots(4, 4)}})
		for _, id := range []int32{fztArrowID, fztDewdropID, fztScrollID} {
			w.give(t, 0, id, 1)
		}
		w.quiet(t)
		frames := w.bypass(t, 0, "enter")
		got := screenFrames(frames[0])
		if len(got) != 1 {
			t.Fatalf("overweight enter = %x, want one SystemMessage", opcodes(got))
		}
		assertStaticSystemMessage(t, got[0], fztOverweight)
		if heard, makers := w.log.take(); len(heard)+len(makers) != 0 {
			t.Fatalf("overweight enter: heard %q, makers %q, want nothing", heard, makers)
		}
	})
	t.Run("restored at stage 3", func(t *testing.T) {
		t.Parallel()
		three := 3
		w := bootTeleporter(t, fztSetup{seats: 1, memo4: &three})
		w.enterPage(t, 0, "frintessa_teleporter007.htm")
		six := w.memoID(t, "6")
		w.direct(t, 0, func(s *script.Script, _ *script.Player) { s.GlobalMemo().Remove("6") })
		w.enterPage(t, 0, "frintessa_teleporter002.htm")
		w.direct(t, 0, func(s *script.Script, _ *script.Player) { s.GlobalMemo().Set("6", int(six)) })
		w.enterPage(t, 0, "frintessa_teleporter007.htm")

		send := func(from string, arg1 int32) {
			id := w.memoID(t, from)
			tp := w.teleporter(t)
			w.direct(t, 0, func(s *script.Script, _ *script.Player) { s.SendScriptEvent(script.NPCOf(tp), id, arg1, 0) })
			w.srv.Settle(t)
		}
		send("5", 0)
		w.enterPage(t, 0, "frintessa_teleporter007.htm")
		send("4", 0)
		w.enterPage(t, 0, "frintessa_teleporter003.htm")
		send("4", 8)
		w.enterPage(t, 0, "frintessa_teleporter007.htm")

		// The minute check: Frintezza at 3 sends the players of the
		// territory out and keeps the stage.
		w.moveTo(t, 0, location.Location{X: 160000, Y: -60000, Z: -2976})
		w.srv.Advance(t, 60*time.Second)
		x, y, _ := w.srv.PlayerPosition(t, w.players[0].id)
		if !near(x, y, 150037, -57255, 150) {
			t.Fatalf("after the minute check at stage 3 the player stands at %d,%d, want near the entrance", x, y)
		}
		w.appear(t, 0)
		w.moveTo(t, 0, fztEntrance)
		w.enterPage(t, 0, "frintessa_teleporter007.htm")

		// Frintezza at 0 with his follower at 0: the stage clears.
		w.setMemo4(t, 0)
		w.moveTo(t, 0, location.Location{X: 160000, Y: -60000, Z: -2976})
		w.srv.Advance(t, 60*time.Second)
		if x, y, _ := w.srv.PlayerPosition(t, w.players[0].id); !near(x, y, 150037, -57255, 150) {
			t.Fatalf("after the minute check at stage 0 the player stands at %d,%d, want near the entrance", x, y)
		}
		w.appear(t, 0)
		w.moveTo(t, 0, fztEntrance)
		w.enterPage(t, 0, "frintessa_teleporter003.htm")
		if heard, makers := w.log.take(); len(heard)+len(makers) != 0 {
			t.Fatalf("heard %q, makers %q, want nothing", heard, makers)
		}
	})
}

// TestFrintezzaTeleporterChannelRefusals walks the channel checks in their
// order: a party in no channel and a member who does not lead it get 003,
// a leader without the scroll 004, three parties 005, six 006, a member out
// of party range 008. The five parties left then enter with Frintezza
// already at stage 2: they are let in, but nobody is told the time and no
// countdown runs.
func TestFrintezzaTeleporterChannelRefusals(t *testing.T) {
	t.Parallel()
	w := bootTeleporter(t, fztSetup{seats: 12})
	for k := range 6 {
		w.invite(t, 2*k, 2*k+1, 0)
	}
	w.enterPage(t, 0, "frintessa_teleporter003.htm")
	for k := 1; k < 3; k++ {
		w.askAndAccept(t, 0, 2*k)
	}
	w.quiet(t)
	w.enterPage(t, 1, "frintessa_teleporter003.htm")
	w.enterPage(t, 2, "frintessa_teleporter003.htm")
	w.enterPage(t, 0, "frintessa_teleporter004.htm")
	w.give(t, 0, fztScrollID, 1)
	w.quiet(t)
	w.enterPage(t, 0, "frintessa_teleporter005.htm")
	for k := 3; k < 6; k++ {
		w.askAndAccept(t, 0, 2*k)
	}
	w.quiet(t)
	w.enterPage(t, 0, "frintessa_teleporter006.htm")
	w.players[11].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	w.srv.Settle(t)
	w.quiet(t)

	w.moveTo(t, 9, location.Location{X: fztEntrance.X + 1600, Y: fztEntrance.Y, Z: fztEntrance.Z})
	w.quiet(t)
	w.enterPage(t, 0, "frintessa_teleporter008.htm")
	w.moveTo(t, 9, fztEntrance)
	w.quiet(t)
	if heard, makers := w.log.take(); len(heard)+len(makers) != 0 {
		t.Fatalf("refusals: heard %q, makers %q, want nothing", heard, makers)
	}

	w.setMemo4(t, 2)
	frames := w.bypass(t, 0, "enter")
	for i := range 10 {
		got := screenFrames(frames[i])
		if slices.Contains(opcodes(got), serverpackets.OpcodeNpcHtmlMessage) && i != 0 {
			t.Fatalf("%s got a page", w.players[i].name)
		}
		if screens := fztScreens(got); len(screens) != 0 {
			t.Fatalf("%s was shown %d screen messages with Frintezza at stage 2", w.players[i].name, len(screens))
		}
		if spots := teleports(got, w.players[i].id); len(spots) != 1 || spots[0] != fztTombSpots[i/2] {
			t.Fatalf("%s teleports = %v, want %v", w.players[i].name, spots, fztTombSpots[i/2])
		}
	}
	if spots := teleports(frames[10], w.players[10].id); len(spots) != 0 {
		t.Fatalf("Seat10, out of the channel, teleports = %v, want none", spots)
	}
	if got := w.srv.PlayerItemCount(t, w.players[0].id, fztScrollID); got != 0 {
		t.Fatalf("Leader's scrolls = %d, want 0", got)
	}
	heard, makers := w.log.take()
	if want := []string{fmt.Sprintf("4 event=%d arg1=1 arg2=0", w.teleporter(t).ObjectID())}; !slices.Equal(heard, want) {
		t.Fatalf("heard %q, want %q", heard, want)
	}
	if len(makers) != 9 {
		t.Fatalf("makers = %q, want nine", makers)
	}
	for i := range 10 {
		w.appear(t, i)
	}
	w.srv.Advance(t, 6*time.Minute)
	for _, p := range w.players {
		if screens := fztScreens(drainFrames(t, p.c)); len(screens) != 0 {
			t.Fatalf("%s was shown a countdown with Frintezza at stage 2", p.name)
		}
	}
}

// TestFrintezzaTeleporterAdmitsAndCountsDown lets a channel of five
// parties in: the makers but wizard 5 told 1001 in order, the leader's
// scroll taken, then party by party each member granted 30 s in the tomb,
// sent to the party's spot and stripped of every Breaking Arrow and
// Dewdrop of Destruction, Frintezza told 1, and the whole channel shown
// that 35 minutes are left. The teleporter then refuses anyone with 007.
// Each party it let in is told every countdown step, as it stands then; a
// party that broke up is told nothing. A minute after the
// last step the tomb resets: the players of its territory sent back out,
// Frintezza and his follower told 0, every maker told 1000, the doors
// sealed. A channel of the four parties left enters again: its parties take
// the first four slots, and the fifth keeps the party it held, which is
// told every step twice. A party keeps being told through its churn: a
// newcomer joins, a step passes, one of the members it entered with
// leaves, and the next step still reaches it; it is still told after two
// more join and the last member seen at a step but its leader leaves. A
// party that disbands is told nothing even once its former members form a
// new party. Frintezza reaching stage 2 ends the countdown (#4287).
func TestFrintezzaTeleporterAdmitsAndCountsDown(t *testing.T) {
	t.Parallel()
	w := bootTeleporter(t, fztSetup{seats: 10})
	w.give(t, 0, fztScrollID, 2)
	for i := range w.players {
		w.give(t, i, fztArrowID, int32(i+1))
		w.give(t, i, fztDewdropID, 3)
	}
	w.quiet(t)
	w.formChannel(t, 5)
	tpID := w.teleporter(t).ObjectID()

	frames := w.bypass(t, 0, "enter")
	var shown [][]byte
	for i, p := range w.players {
		got := screenFrames(frames[i])
		if spots := teleports(got, p.id); len(spots) != 1 || spots[0] != fztTombSpots[i/2] {
			t.Fatalf("%s teleports = %v, want %v", p.name, spots, fztTombSpots[i/2])
		}
		shown = append(shown, requireScreenMessage(t, p.name, fztScreens(got), fztRemaining(35), 10000))
		for _, id := range []int32{fztArrowID, fztDewdropID} {
			if n := w.srv.PlayerItemCount(t, p.id, id); n != 0 {
				t.Fatalf("%s keeps %d of item %d, want none", p.name, n, id)
			}
		}
	}
	for _, f := range shown[1:] {
		if !slices.Equal(f, shown[0]) {
			t.Fatalf("screen messages differ: % x and % x", shown[0], f)
		}
	}
	if got := w.srv.PlayerItemCount(t, w.players[0].id, fztScrollID); got != 1 {
		t.Fatalf("Leader's scrolls = %d, want 1", got)
	}
	var ids []int32
	for _, p := range w.players {
		ids = append(ids, p.id)
	}
	slices.Sort(ids)
	allowed := w.tomb.AllowedPlayers()
	slices.Sort(allowed)
	if !slices.Equal(allowed, ids) {
		t.Fatalf("allowed in the tomb = %v, want %v", allowed, ids)
	}
	heard, makers := w.log.take()
	if want := []string{fmt.Sprintf("4 event=%d arg1=1 arg2=0", tpID)}; !slices.Equal(heard, want) {
		t.Fatalf("heard %q, want %q", heard, want)
	}
	var want1001 []string
	for _, m := range fztMakers {
		if m != "godard32_2515_08m1" {
			want1001 = append(want1001, m+" 1001")
		}
	}
	if !slices.Equal(makers, want1001) {
		t.Fatalf("makers = %q, want %q", makers, want1001)
	}
	for i := range w.players {
		w.appear(t, i)
	}
	// Frintezza takes stage 1 as he is told, which the minute check leaves.
	w.setMemo4(t, 1)
	w.moveTo(t, 0, fztEntrance)
	w.enterPage(t, 0, "frintessa_teleporter007.htm")

	// The second party breaks up.
	w.players[3].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	w.srv.Settle(t)
	w.quiet(t)

	// countdownTo advances wait and checks that seat i is shown the step
	// told[i] times.
	countdownTo := func(minutes int, wait time.Duration, told map[int]int) {
		t.Helper()
		w.srv.Advance(t, wait)
		for i, p := range w.players {
			screens := fztScreens(drainFrames(t, p.c))
			if want := told[i]; len(screens) != want {
				t.Fatalf("%d minutes: %s shown %d messages, want %d", minutes, p.name, len(screens), want)
			}
			for _, f := range screens {
				requireScreenMessage(t, p.name, [][]byte{f}, fztRemaining(minutes), 10000)
			}
		}
	}
	// countdown is countdownTo with both seats of each pair in told told
	// once per mention.
	countdown := func(minutes int, wait time.Duration, told []int) {
		t.Helper()
		seats := map[int]int{}
		for _, j := range told {
			seats[2*j]++
			seats[2*j+1]++
		}
		countdownTo(minutes, wait, seats)
	}
	steps := []struct {
		minutes int
		wait    time.Duration
	}{{30, 5 * time.Minute}, {25, 5 * time.Minute}, {20, 5 * time.Minute}, {15, 5 * time.Minute}, {10, 5 * time.Minute}, {5, 5 * time.Minute}, {4, time.Minute}, {3, time.Minute}, {2, time.Minute}, {1, time.Minute}}
	for _, s := range steps {
		countdown(s.minutes, s.wait, []int{0, 2, 3, 4})
	}
	if heard, makers := w.log.take(); len(heard)+len(makers) != 0 {
		t.Fatalf("during the countdown: heard %q, makers %q, want nothing", heard, makers)
	}

	w.srv.Advance(t, time.Minute)
	for i, p := range w.players {
		x, y, _ := w.srv.PlayerPosition(t, p.id)
		if !near(x, y, fztEntrance.X, fztEntrance.Y, 150) {
			t.Fatalf("after the reset %s stands at %d,%d, want near the entrance", p.name, x, y)
		}
		w.appear(t, i)
	}
	heard, makers = w.log.take()
	if want := []string{fmt.Sprintf("4 event=%d arg1=0 arg2=0", tpID), fmt.Sprintf("5 event=%d arg1=0 arg2=0", tpID)}; !slices.Equal(heard, want) {
		t.Fatalf("reset heard %q, want %q", heard, want)
	}
	var want1000 []string
	for _, m := range fztMakers {
		want1000 = append(want1000, m+" 1000")
	}
	if !slices.Equal(makers, want1000) {
		t.Fatalf("reset makers = %q, want %q", makers, want1000)
	}
	w.wantDoors(t, "after the reset", func(i int) bool { return i == 2 || i == 3 })

	// The four parties left enter again.
	w.moveTo(t, 0, fztEntrance)
	frames = w.bypass(t, 0, "enter")
	order := []int{0, 2, 3, 4} // the channel's parties, by the seats' pairs
	for slot, pair := range order {
		for _, i := range []int{2 * pair, 2*pair + 1} {
			if spots := teleports(frames[i], w.players[i].id); len(spots) != 1 || spots[0] != fztTombSpots[slot] {
				t.Fatalf("second entry: %s teleports = %v, want %v", w.players[i].name, spots, fztTombSpots[slot])
			}
		}
	}
	for i := range w.players {
		w.appear(t, i)
	}
	w.log.take()
	w.setMemo4(t, 1)
	// Slots 0 to 3 hold the pairs 0, 2, 3, 4; slot 4 still holds pair 4.
	countdown(30, 5*time.Minute, []int{0, 2, 3, 4, 4})
	// Seat 3 joins the first party, and the step after reaches it whole.
	w.invite(t, 0, 3, 0)
	countdownTo(25, 5*time.Minute, map[int]int{0: 1, 1: 1, 3: 1, 4: 1, 5: 1, 6: 1, 7: 1, 8: 2, 9: 2})
	// Seat 1, one of the members it was stored with, leaves: the party
	// lives on with seats 0 and 3 and is still told.
	w.players[1].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	w.srv.Settle(t)
	w.quiet(t)
	countdownTo(20, 5*time.Minute, map[int]int{0: 1, 3: 1, 4: 1, 5: 1, 6: 1, 7: 1, 8: 2, 9: 2})
	// Seats 1 and 2 join the first party and seat 3 leaves it: of the
	// members it was told as, only its leader is left, and it is told whole.
	w.invite(t, 0, 1, 0)
	w.invite(t, 0, 2, 0)
	w.players[3].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	w.srv.Settle(t)
	w.quiet(t)
	// The fourth party breaks up and its two players form a new party,
	// which is not the one let in.
	w.players[7].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	w.srv.Settle(t)
	w.quiet(t)
	w.invite(t, 6, 7, 0)
	countdownTo(15, 5*time.Minute, map[int]int{0: 1, 1: 1, 2: 1, 4: 1, 5: 1, 8: 2, 9: 2})
	w.setMemo4(t, 2)
	countdown(10, 5*time.Minute, nil)
	w.srv.Advance(t, 40*time.Minute)
	if heard, makers := w.log.take(); len(heard)+len(makers) != 0 {
		t.Fatalf("after the countdown ended: heard %q, makers %q, want nothing", heard, makers)
	}
}
