package ai

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/sirra"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
	"github.com/rs/zerolog"
)

// Freya's deacon, a civilian that logs the script events it hears (the
// global memo's creature 7), Silver Hemocyte, the item the paying leader
// receives, an item that fills an inventory slot, and the two ice
// barriers.
const (
	sirraDeaconID   = int32(32029)
	sirraListenerID = int32(32031)
	sirraHemocyte   = int32(8057)
	sirraReward     = int32(8379)
	sirraFiller     = int32(8999)
	sirraBarrier1   = 23140001
	sirraBarrier2   = 23140002
	sirraDirector   = "SirraDirector"
)

// sirraZones are the three garden zones the deacon lets a party into.
var sirraZones = []int{110013, 110014, 110015}

// sirraAtDeacon is where the character enters the world, east of the
// deacon; sirraGarden where the deacon sends the party.
var (
	sirraAtDeacon = location.Location{X: 115500, Y: -125706, Z: -3388}
	sirraGarden   = location.Location{X: 113533, Y: -126159, Z: -3488}
)

// sirraSpawnlist is the shipped deacon npcmaker, the boss npcmaker and the
// keeper npcmaker as probes that log the maker script events they run and
// spawn nothing, and the listener 500 north of the deacon.
const sirraSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="schuttgart13_npc2314_1" minZ="-3488" maxZ="-3288">
		<node x="115351" y="-125806"/>
		<node x="115551" y="-125806"/>
		<node x="115551" y="-125606"/>
		<node x="115351" y="-125606"/>
	</territory>
	<npcmaker name="schuttgart13_npc2314_1m1" territory="schuttgart13_npc2314_1" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="32029" pos="115451;-125706;-3388;0" total="1" respawn="1min"/>
	</npcmaker>
	<npcmaker name="schuttgart13_mb2314_05m1" territory="schuttgart13_npc2314_1" maximumNpcs="1">
		<ai type="sirra_probe"/>
	</npcmaker>
	<npcmaker name="schuttgart13_npc2314_3m1" territory="schuttgart13_npc2314_1" maximumNpcs="1">
		<ai type="sirra_probe"/>
	</npcmaker>
	<npcmaker name="sirra_listener" territory="schuttgart13_npc2314_1" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="32031" pos="115451;-125206;-3388;0" total="1" respawn="1min"/>
	</npcmaker>
</list>`

// sirraWorld is the character beside Freya's deacon, with the log of the
// probes and the listener.
type sirraWorld struct {
	*baiumWorld
	log   *sailrenLog
	zones []*zone.Boss
}

// sirraPages are the deacon's pages, each naming itself, with the links of
// the shipped pages; the Silver Hemocyte refusal keeps its %name%.
func sirraPages() map[string]string {
	links := map[string][]string{
		"freya_deacon001.htm":       {"enter"},
		"freya_deacon002.htm":       {"link_07", "link_08"},
		"freya_deacon_q0656_01.htm": nil,
		"freya_deacon_q0656_02.htm": nil,
		"freya_deacon_q0656_05.htm": {"link_08"},
		"freya_deacon_q0656_06.htm": nil,
		"freya_deacon_q0656_07.htm": {"link_08"},
		"freya_deacon_q0656_08.htm": {"link_09"},
	}
	pages := map[string]string{
		"script/ai/boss/sirra/FreyaDeacon/freya_deacon_q0656_03.htm": "<html><body>freya_deacon_q0656_03.htm %name% lacks Silver Hemocytes</body></html>",
	}
	for name, events := range links {
		var b strings.Builder
		b.WriteString("<html><body>" + name)
		for _, e := range events {
			b.WriteString(`<a action="bypass -h Quest FreyaDeacon ` + e + `">` + e + "</a>")
		}
		b.WriteString("</body></html>")
		pages["script/ai/boss/sirra/FreyaDeacon/"+name] = b.String()
	}
	return pages
}

// sirraBarrier is one of the two ice barriers, open as shipped.
func sirraBarrier(id int, name string, at location.Location) *door.Template {
	return &door.Template{
		ID: id, Name: name, Kind: door.KindDoor, Level: 1, Opened: true,
		Position:    at,
		Coordinates: []location.Point{{X: at.X - 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y - 100}, {X: at.X + 50, Y: at.Y + 100}, {X: at.X - 50, Y: at.Y + 100}},
		HP:          126600, PDef: 644, MDef: 518, Height: 1024,
	}
}

// bootSirra boots the deacon's hall with the character entering the world
// beside the deacon holding hemocytes Silver Hemocytes, the global memo's
// creature 7 the listener, and the deacon as scripts.xml lists it. opts are
// added last.
func bootSirra(t *testing.T, hemocytes int32, opts ...gameservertest.Option) *sirraWorld {
	t.Helper()
	w := &sirraWorld{baiumWorld: &baiumWorld{start: time.Now()}, log: &sailrenLog{}}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sirra.xml"), []byte(sirraSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	for i, id := range sirraZones {
		// The first zone holds the garden's entry; the other two lie apart.
		x := 113000 - 5000*i
		form, err := zone.NewCuboid(x, x+1000, -126700, -125600, -4000, -3000)
		if err != nil {
			t.Fatal(err)
		}
		z := zone.NewBoss(id, form)
		zones.Add(z)
		w.zones = append(w.zones, z)
	}
	makers := script.MakerCatalog{
		"sirra_probe": func() script.Maker {
			return script.Maker{MakerHooks: script.MakerHooks{
				OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
					w.log.add(fmt.Sprintf("maker %s %s %d %d", e.Group.Maker().Name, e.Name, e.Int1, e.Int2))
				},
			}}
		},
	}
	kinds := map[int32]script.NPCKind{sirraDeaconID: script.KindFolk, sirraListenerID: script.KindFolk}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.sirra.FreyaDeacon"},
		{Path: "test.SirraListener"},
		{Path: "test." + sirraDirector},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":  defaultnpc.New,
		"script.ai.boss.sirra.FreyaDeacon": sirra.FreyaDeacon,
		"test.SirraListener": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{sirraListenerID}, Hooks: script.Hooks{
				OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
					w.log.add(fmt.Sprintf("memo heard %d %d %d", e.EventID, e.Arg1, e.Arg2))
				},
			}}
		},
		"test." + sirraDirector: func() script.Script { return script.Script{} },
	}
	etc := func(id int32, name string, weight int32) *item.Template {
		return &item.Template{
			ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: true, Weight: weight,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}
	}
	items := append(gameservertest.ItemTemplates().All(),
		etc(sirraHemocyte, "Silver Hemocyte", 0), etc(sirraReward, "Reward", 0), etc(sirraFiller, "Filler", 0))
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{sirraAtDeacon}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", int(sirraDeaconID)), gameservertest.FolkTemplate("Folk", int(sirraListenerID)),
		})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithMakers(makers),
		gameservertest.WithDoors(
			sirraBarrier(sirraBarrier1, "ice_barrier_001", location.Location{X: 113932, Y: -126022, Z: -3456}),
			sirraBarrier(sirraBarrier2, "ice_barrier_002", location.Location{X: 115292, Y: -125745, Z: -3456}),
		),
		gameservertest.WithZones(zones),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
		gameservertest.WithHTMLPages(sirraPages()),
		gameservertest.WithItemTemplates(item.NewTable(items)),
	}, opts...)...)
	if hemocytes > 0 {
		srv.GiveItem(t, srv.SoleObjectID(t), sirraHemocyte, hemocytes)
	}
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	w.warriorWorld = &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}

	listener := soleBaiumNPC(t, w.baiumWorld, sirraListenerID)
	w.srv.RunScript(t, id, sirraDirector, func(s *script.Script, _ *script.Player) {
		s.GlobalMemo().Set("7", int(listener.ObjectID()))
	})
	return w
}

// join brings the character name, of account, into the world beside the
// character and into the party the character leads, holding hemocytes
// Silver Hemocytes.
func (w *sirraWorld) join(t *testing.T, account, name string, hemocytes int32) sailrenMate {
	t.Helper()
	m := sailrenMate{id: w.srv.SeedCharacterFor(t, account, name, 20, 0).ObjectID()}
	if hemocytes > 0 {
		w.srv.GiveItem(t, m.id, sirraHemocyte, hemocytes)
	}
	m.c = w.srv.DialClient(t, account, 1)
	m.c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatalf("no CharSelected for %s within 100 frames", name)
		}
		if m.c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	m.c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	w.srv.ReadQueued(t, m.c)
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Client.Send(encodeJoinParty(name))
	w.srv.ReadQueued(t, m.c)
	m.c.Send(encodeAnswerJoinParty(1))
	w.srv.ReadQueued(t, m.c)
	w.srv.ReadQueued(t, w.srv.Client)
	members := make(chan int, 1)
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() { members <- len(script.PlayerOf(w.player).PartyMembers()) }) {
		t.Fatal("the character's queue is closed")
	}
	if <-members < 2 {
		t.Fatalf("the character leads no party after %s accepted", name)
	}
	return m
}

// place teleports the online player id to at and lets its client appear
// there.
func (w *sirraWorld) place(t *testing.T, id int32, c *testsupport.ScriptedClient, at location.Location) {
	t.Helper()
	w.srv.RunScript(t, id, sirraDirector, func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0)
	})
	w.srv.Settle(t)
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, c)
	if x, y, _ := w.srv.PlayerPosition(t, id); x != at.X || y != at.Y {
		t.Fatalf("setup: player %d stands at %d,%d, want %d,%d", id, x, y, at.X, at.Y)
	}
}

// talk selects the deacon, unless c already has, and talks to it through
// c, and returns the frames sent.
func (w *sirraWorld) talk(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	deacon := soleBaiumNPC(t, w.baiumWorld, sirraDeaconID)
	var frames [][]byte
	for range 2 {
		c.Send(encodeFolkAction(deacon.ObjectID(), w.at))
		w.srv.Settle(t)
		frames = append(frames, w.srv.ReadQueued(t, c)...)
		if len(baiumPageNames(frames)) > 0 {
			break
		}
	}
	return frames
}

// bypass asks event of the deacon through c and returns the frames sent.
func (w *sirraWorld) bypass(t *testing.T, c *testsupport.ScriptedClient, event string) [][]byte {
	t.Helper()
	c.Send(encodeManagerBypass("Quest FreyaDeacon " + event))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, c)
}

// wantFirstPage fails unless talking to the deacon through c answers page.
func (w *sirraWorld) wantFirstPage(t *testing.T, c *testsupport.ScriptedClient, page string) {
	t.Helper()
	if got := baiumPageNames(w.talk(t, c)); !slices.Equal(got, []string{page}) {
		t.Fatalf("first talk: pages = %q, want %q", got, page)
	}
}

// scriptEvent sends the deacon script event eventID, from the director on
// the character's queue.
func (w *sirraWorld) scriptEvent(t *testing.T, eventID int32) {
	t.Helper()
	n := soleBaiumNPC(t, w.baiumWorld, sirraDeaconID)
	if !w.srv.RunScript(t, w.player.ObjectID(), sirraDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(n), eventID, 0, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// wantBarriers fails unless both ice barriers stand open, or both closed.
func (w *sirraWorld) wantBarriers(t *testing.T, step string, open bool) {
	t.Helper()
	for _, id := range []int{sirraBarrier1, sirraBarrier2} {
		if _, got := w.srv.DoorState(t, id); got != open {
			t.Fatalf("%s: barrier %d open = %v, want %v", step, id, got, open)
		}
	}
}

// deaconMemory renders the deacon's script memory: its leader, busy and
// third marks, the party size it keeps, and whether it keeps a leader.
func (w *sirraWorld) deaconMemory(t *testing.T) string {
	t.Helper()
	n := script.NPCOf(soleBaiumNPC(t, w.baiumWorld, sirraDeaconID))
	return fmt.Sprintf("leader=%d busy=%d third=%d count=%d entrant=%v",
		n.Int(script.SlotIntAI1), n.Int(script.SlotIntAI2), n.Int(script.SlotIntAI3), n.Int(script.SlotIntAI4),
		n.Slot(script.SlotCreature0) != nil)
}

// sirraFrames renders, in order, a step's pages, screen messages, NPC
// chat, teleports, system messages and released clients.
func sirraFrames(t *testing.T, frames [][]byte, roles map[int32]string) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeNpcHtmlMessage:
			out = append(out, "S NpcHtmlMessage "+strings.TrimSuffix(strings.Join(managerHTML([][]byte{f}), ""), "\n"))
		case f[0] == serverpackets.OpcodeExtended:
			for _, text := range screenTexts([][]byte{f}) {
				out = append(out, "S ExShowScreenMessage "+text)
			}
		case f[0] == serverpackets.OpcodeActionFailed || f[0] == 0x64 || f[0] == serverpackets.OpcodeNpcSay || f[0] == serverpackets.OpcodeTeleportToLocation:
			line, err := scriptcontract.Packet(f, roles)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, line)
		}
	}
	return out
}

// wantSirraFrames fails unless the step's rendered frames are want.
func wantSirraFrames(t *testing.T, step string, frames [][]byte, roles map[int32]string, want ...string) {
	t.Helper()
	if got := sirraFrames(t, frames, roles); !slices.Equal(got, want) {
		t.Fatalf("%s frames:\n got %q\nwant %q", step, got, want)
	}
}

// sirraPage renders the shown page name as sirraFrames does, with the
// links the fixture page carries.
func sirraPage(name string) string {
	return "S NpcHtmlMessage " + sirraPages()["script/ai/boss/sirra/FreyaDeacon/"+name]
}

// screen renders the NpcString id as a screen message shown for ms.
func screen(id int32, ms int) string {
	return fmt.Sprintf("S ExShowScreenMessage %s@%d", script.NpcString(id), ms)
}

// shout renders the deacon's shout of the NpcString id.
func shout(id int32) string {
	return fmt.Sprintf("S NpcSay obj=deacon type=1 npc=%d text=%s", 1000000+sirraDeaconID, scriptcontract.Quote(script.NpcString(id)))
}

// TestFreyaDeaconTurnsAwayTheUnready ports the deacon's refusals (phase
// 1): free, it greets with freya_deacon001; a player in no party is told to
// form one (06); a member who does not lead is turned away (02); a leader
// whose member lacks 10 Silver Hemocytes gets nothing while that member is
// shown the refusal with its own name, its client released; and an
// overweight leader is told to lighten its inventory. Nothing is taken,
// and the deacon stays free.
func TestFreyaDeaconTurnsAwayTheUnready(t *testing.T) {
	// Two inventory slots: the leader's Silver Hemocytes fill one, and a
	// second item makes the inventory full enough to be overweight.
	w := bootSirra(t, 10, gameservertest.WithInventorySlots(2, 2))
	c := w.srv.Client
	roles := map[int32]string{w.player.ObjectID(): "player"}
	w.wantFirstPage(t, c, "freya_deacon001.htm")
	wantSirraFrames(t, "enter alone", w.bypass(t, c, "enter"), roles, sirraPage("freya_deacon_q0656_06.htm"), "S ActionFailed")

	mate := w.join(t, "mate", "Mate", 0)
	roles[mate.id] = "mate"
	w.wantFirstPage(t, mate.c, "freya_deacon001.htm")
	wantSirraFrames(t, "a member enters", w.bypass(t, mate.c, "enter"), roles, sirraPage("freya_deacon_q0656_02.htm"), "S ActionFailed")

	w.wantFirstPage(t, c, "freya_deacon001.htm")
	wantSirraFrames(t, "the leader enters, Mate short", w.bypass(t, c, "enter"), roles)
	wantSirraFrames(t, "the leader enters, to Mate", w.srv.ReadQueued(t, mate.c), roles,
		"S NpcHtmlMessage <html><body>freya_deacon_q0656_03.htm Mate lacks Silver Hemocytes</body></html>", "S ActionFailed")

	w.srv.RunScript(t, mate.id, sirraDirector, func(s *script.Script, p *script.Player) { s.GiveItems(p, sirraHemocyte, 10) })
	w.srv.RunScript(t, w.player.ObjectID(), sirraDirector, func(s *script.Script, p *script.Player) { s.GiveItems(p, sirraFiller, 1) })
	w.srv.Settle(t)
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, mate.c)
	w.wantFirstPage(t, c, "freya_deacon001.htm")
	wantSirraFrames(t, "the overweight leader enters", w.bypass(t, c, "enter"), roles, "S SystemMessage id=1118")

	for _, id := range []int32{w.player.ObjectID(), mate.id} {
		if got := w.srv.PlayerItemCount(t, id, sirraHemocyte); got != 10 {
			t.Fatalf("player %d holds %d Silver Hemocytes after the refusals, want 10", id, got)
		}
	}
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=0 count=0 entrant=false"; got != want {
		t.Fatalf("the deacon's memory = %s, want %s", got, want)
	}
	w.log.expect(t, "the refusals")
}

// TestFreyaDeaconLetsThePartyIn ports the entry (phases 1 to 3): the leader
// pays 10 Silver Hemocytes per member, the party is told so for 100 s, the
// leader receives three of item 8379 and the 05 page, and the deacon
// shouts. Busy, it answers the party with 002 and anyone else with 01. Two
// minutes later the memo's creature hears 10027 and both barriers close;
// 5 s later the party is told the gate opens, every member within 1000 may
// enter the three garden zones and is teleported in, and the boss maker
// runs 11040 with the leader. Event 10005 then runs 10025 on the keeper
// maker, tells the party it is over, sends 11039 to the NPCs within 8000
// and opens both barriers, and the deacon is free again.
func TestFreyaDeaconLetsThePartyIn(t *testing.T) {
	w := bootSirra(t, 10)
	c := w.srv.Client
	mate := w.join(t, "mate", "Mate", 10)
	deacon := soleBaiumNPC(t, w.baiumWorld, sirraDeaconID)
	roles := map[int32]string{w.player.ObjectID(): "player", mate.id: "mate", deacon.ObjectID(): "deacon"}

	w.wantFirstPage(t, c, "freya_deacon001.htm")
	w.srv.ReadQueued(t, mate.c)
	wantSirraFrames(t, "enter", w.bypass(t, c, "enter"), roles,
		"S SystemMessage id=301 3:8057 6:10", screen(1121000, 100000), "S SystemMessage id=53 3:8379 6:3", shout(1121005),
		sirraPage("freya_deacon_q0656_05.htm"), "S ActionFailed")
	wantSirraFrames(t, "enter, Mate", w.srv.ReadQueued(t, mate.c), roles,
		"S SystemMessage id=301 3:8057 6:10", screen(1121000, 100000), shout(1121005))
	for _, id := range []int32{w.player.ObjectID(), mate.id} {
		if got := w.srv.PlayerItemCount(t, id, sirraHemocyte); got != 0 {
			t.Fatalf("player %d holds %d Silver Hemocytes after the entry, want 0", id, got)
		}
	}
	if got := w.srv.PlayerItemCount(t, w.player.ObjectID(), sirraReward); got != 3 {
		t.Fatalf("the leader holds %d of item 8379, want 3", got)
	}
	if got, want := w.deaconMemory(t), fmt.Sprintf("leader=%d busy=1 third=0 count=2 entrant=true", w.player.ObjectID()); got != want {
		t.Fatalf("the deacon's memory = %s, want %s", got, want)
	}

	w.wantFirstPage(t, c, "freya_deacon002.htm")
	wantSirraFrames(t, "link_07", w.bypass(t, c, "link_07"), roles, sirraPage("freya_deacon_q0656_07.htm"), "S ActionFailed")
	wantSirraFrames(t, "link_08", w.bypass(t, c, "link_08"), roles, sirraPage("freya_deacon_q0656_08.htm"), "S ActionFailed")
	w.wantFirstPage(t, mate.c, "freya_deacon002.htm")
	wantSirraFrames(t, "enter while busy", w.bypass(t, c, "enter"), roles)
	w.srv.ReadQueued(t, mate.c)

	w.advance(t, 119*time.Second)
	w.log.expect(t, "119 s after the entry")
	w.wantBarriers(t, "119 s after the entry", true)
	w.advance(t, time.Second)
	w.log.expect(t, "two minutes after the entry", "memo heard 10027 0 0")
	w.wantBarriers(t, "two minutes after the entry", false)
	w.srv.ReadQueued(t, mate.c)

	frames := w.advance(t, 5*time.Second)
	w.log.expect(t, "the gate", fmt.Sprintf("maker schuttgart13_mb2314_05m1 11040 %d 0", w.player.ObjectID()))
	wantSirraFrames(t, "the gate", noReleases(ownTeleports(frames, w.player.ObjectID())), roles,
		screen(1121001, 10000), "S TeleportToLocation obj=player loc=113533,-126159,-3488 fast=0")
	wantSirraFrames(t, "the gate, Mate", noReleases(ownTeleports(w.srv.ReadQueued(t, mate.c), mate.id)), roles,
		screen(1121001, 10000), "S TeleportToLocation obj=mate loc=113533,-126159,-3488 fast=0")
	want := []int32{w.player.ObjectID(), mate.id}
	slices.Sort(want)
	for i, z := range w.zones {
		allowed := z.AllowedPlayers()
		slices.Sort(allowed)
		if !slices.Equal(allowed, want) {
			t.Fatalf("players zone %d admits = %v, want %v", sirraZones[i], allowed, want)
		}
	}
	for _, mc := range []*testsupport.ScriptedClient{c, mate.c} {
		mc.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
		w.srv.Settle(t)
	}
	for _, id := range want {
		if x, y, z := w.srv.PlayerPosition(t, id); x != sirraGarden.X || y != sirraGarden.Y || z != sirraGarden.Z {
			t.Fatalf("player %d stands at %d,%d,%d after the gate, want the garden's entry", id, x, y, z)
		}
	}
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, mate.c)

	w.scriptEvent(t, 10005)
	w.log.expect(t, "10005", "maker schuttgart13_npc2314_3m1 10025 0 0", "memo heard 11039 0 0")
	wantSirraFrames(t, "10005", w.srv.ReadQueued(t, c), roles, screen(1121003, 10000))
	w.wantBarriers(t, "10005", true)
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=0 count=2 entrant=false"; got != want {
		t.Fatalf("the deacon's memory after 10005 = %s, want %s", got, want)
	}
	w.place(t, w.player.ObjectID(), c, sirraAtDeacon)
	w.wantFirstPage(t, c, "freya_deacon001.htm")
}

// TestFreyaDeaconTurnsAwayAGrownParty ports the gate's refusal (phases 1
// and 2): a party that grew after it paid is not let in; the deacon shouts,
// opens both barriers and is free again, and the boss maker runs nothing.
// A member who left the party's side is never let in either.
func TestFreyaDeaconTurnsAwayAGrownParty(t *testing.T) {
	w := bootSirra(t, 10)
	c := w.srv.Client
	mate := w.join(t, "mate", "Mate", 10)
	deacon := soleBaiumNPC(t, w.baiumWorld, sirraDeaconID)
	roles := map[int32]string{w.player.ObjectID(): "player", mate.id: "mate", deacon.ObjectID(): "deacon"}
	w.wantFirstPage(t, c, "freya_deacon001.htm")
	w.bypass(t, c, "enter")
	third := w.join(t, "third", "Third", 0)
	roles[third.id] = "third"
	w.wantFirstPage(t, third.c, "freya_deacon002.htm")
	w.srv.ReadQueued(t, c)

	w.advance(t, 120*time.Second)
	w.log.expect(t, "two minutes after the entry", "memo heard 10027 0 0")
	w.wantBarriers(t, "two minutes after the entry", false)
	w.srv.ReadQueued(t, third.c)
	wantSirraFrames(t, "the gate", w.advance(t, 5*time.Second), roles, screen(1121001, 10000), shout(1121007))
	w.log.expect(t, "the gate")
	w.wantBarriers(t, "the gate", true)
	for _, z := range w.zones {
		if allowed := z.AllowedPlayers(); len(allowed) != 0 {
			t.Fatalf("the zones admit %v after the refusal, want none", allowed)
		}
	}
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=0 count=2 entrant=false"; got != want {
		t.Fatalf("the deacon's memory after the refusal = %s, want %s", got, want)
	}
	w.wantFirstPage(t, third.c, "freya_deacon001.htm")
}

// TestFreyaDeaconEndsAVisitOn10026 ports event 10026 (phase 3): the party
// is told the visit is over, the NPCs within 8000 hear 11039, both
// barriers open, the deacon marks its third slot and is free. Its timers
// still run: two minutes after the entry the memo's creature hears 10027,
// both barriers close and the mark clears, and with no leader kept the gate
// does nothing. Event 11037 clears the mark too.
func TestFreyaDeaconEndsAVisitOn10026(t *testing.T) {
	w := bootSirra(t, 10)
	c := w.srv.Client
	mate := w.join(t, "mate", "Mate", 10)
	deacon := soleBaiumNPC(t, w.baiumWorld, sirraDeaconID)
	roles := map[int32]string{w.player.ObjectID(): "player", mate.id: "mate", deacon.ObjectID(): "deacon"}
	w.wantFirstPage(t, c, "freya_deacon001.htm")
	w.bypass(t, c, "enter")
	w.srv.ReadQueued(t, mate.c)

	w.scriptEvent(t, 10026)
	w.log.expect(t, "10026", "memo heard 11039 0 0")
	wantSirraFrames(t, "10026", w.srv.ReadQueued(t, c), roles, screen(1121004, 10000))
	wantSirraFrames(t, "10026, Mate", w.srv.ReadQueued(t, mate.c), roles, screen(1121004, 10000))
	w.wantBarriers(t, "10026", true)
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=1 count=2 entrant=false"; got != want {
		t.Fatalf("the deacon's memory after 10026 = %s, want %s", got, want)
	}

	w.advance(t, 120*time.Second)
	w.log.expect(t, "two minutes after the entry", "memo heard 10027 0 0")
	w.wantBarriers(t, "two minutes after the entry", false)
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=0 count=2 entrant=false"; got != want {
		t.Fatalf("the deacon's memory after timer 1005 = %s, want %s", got, want)
	}
	wantSirraFrames(t, "the gate", w.advance(t, 5*time.Second), roles)
	w.log.expect(t, "the gate")
	w.wantBarriers(t, "the gate", false)

	w.scriptEvent(t, 10026)
	w.log.expect(t, "10026 again", "memo heard 11039 0 0")
	w.scriptEvent(t, 11037)
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=0 count=2 entrant=false"; got != want {
		t.Fatalf("the deacon's memory after 11037 = %s, want %s", got, want)
	}
}

// ownTeleports drops from frames the teleports of anyone but id, which
// reach a client in the order other players' queues run them.
func ownTeleports(frames [][]byte, id int32) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation && int32(binary.LittleEndian.Uint32(f[1:5])) != id {
			continue
		}
		out = append(out, f)
	}
	return out
}

// noReleases drops from frames the ActionFailed releases a teleport sends
// on its own.
func noReleases(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeActionFailed {
			out = append(out, f)
		}
	}
	return out
}
