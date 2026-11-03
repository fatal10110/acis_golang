package ai

import (
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
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/sailren"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// Shilen's Stone Statue, Sailren's lair dummy, the teleport cube the dummy
// leaves, Sailren (here a civilian that only logs the script events it
// hears), Gazkh, and the nest's grand boss zone.
const (
	sailrenStatueID   = int32(32109)
	sailrenDummyID    = int32(32110)
	sailrenCubeID     = int32(32107)
	sailrenListenerID = int32(29065)
	sailrenGazkh      = int32(8784)
	sailrenNestZone   = 110011
	sailrenDirector   = "SailrenDirector"
)

// sailrenAtStatue is where the character enters the world beside the
// statue; sailrenInNest where it enters the world inside the dummy's
// territory, beside it.
var (
	sailrenAtStatue = location.Location{X: 23700, Y: -8464, Z: -1345}
	sailrenInNest   = location.Location{X: 27600, Y: -6640, Z: -2009}
)

// sailrenSpawnlist is the shipped statue and dummy npcmakers with their
// territories, the three dinosaur npcmakers as probes that log the maker
// script events they run and spawn nothing, and Sailren 240 north of the
// dummy.
const sailrenSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rune16_npc2017_01" minZ="-1388" maxZ="-1188">
		<node x="23564" y="-8564"/>
		<node x="23764" y="-8564"/>
		<node x="23764" y="-8364"/>
		<node x="23564" y="-8364"/>
	</territory>
	<npcmaker name="rune16_npc2017_01m1" territory="rune16_npc2017_01" maximumNpcs="1">
		<ai type="statue_of_shilen_maker"/>
		<npc id="32109" pos="23664;-8464;-1345;33000" total="1" respawn="1min"/>
	</npcmaker>
	<territory name="rune16_npc2017_13" minZ="-2928" maxZ="-928">
		<node x="27648" y="-8688"/>
		<node x="29320" y="-7432"/>
		<node x="29040" y="-5292"/>
		<node x="25736" y="-4796"/>
		<node x="25748" y="-7996"/>
	</territory>
	<npcmaker name="rune16_npc2017_13m1" territory="rune16_npc2017_13" maximumNpcs="2">
		<ai type="sailren_dummy_maker"/>
		<npc id="32110" pos="27549;-6640;-2009;0" total="1" respawn="1min"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_01m1" territory="rune16_npc2017_13" maximumNpcs="3">
		<ai type="sailren_probe"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_02m1" territory="rune16_npc2017_13" maximumNpcs="1">
		<ai type="sailren_probe"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_03m1" territory="rune16_npc2017_13" maximumNpcs="1">
		<ai type="sailren_probe"/>
	</npcmaker>
	<npcmaker name="sailren_listener" territory="rune16_npc2017_13" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="29065" pos="27549;-6400;-2009;0" total="1" respawn="1min"/>
	</npcmaker>
</list>`

// sailrenLog records, in order, the maker script events the probe
// npcmakers run and the script events the listener hears.
type sailrenLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *sailrenLog) add(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line)
}

// take returns the lines recorded since the last take.
func (l *sailrenLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

// expect fails unless the lines recorded since the last take are want.
func (l *sailrenLog) expect(t *testing.T, step string, want ...string) {
	t.Helper()
	if got := l.take(); !slices.Equal(got, want) {
		t.Fatalf("%s: logged\n got %q\nwant %q", step, got, want)
	}
}

// sailrenWorld is the character on Sailren's island, with the log of the
// probes and the listener.
type sailrenWorld struct {
	*baiumWorld
	log *sailrenLog
}

// sailrenPages are the statue's pages, each naming itself, with the links
// of the shipped pages.
func sailrenPages() map[string]string {
	links := map[string][]string{
		"party_range.htm":          nil,
		"statue_of_shilen002.htm":  {"battle_prep"},
		"statue_of_shilen002a.htm": {"enter_details"},
		"statue_of_shilen003.htm":  {"battle_prep"},
		"statue_of_shilen003a.htm": {"learn_more"},
		"statue_of_shilen004.htm":  {"battle_prep"},
		"statue_of_shilen004a.htm": nil,
		"statue_of_shilen005.htm":  {"battle_prep"},
		"statue_of_shilen006.htm":  {"battle_prep"},
	}
	page := func(name string, events []string) string {
		var b strings.Builder
		b.WriteString("<html><body>" + name)
		for _, e := range events {
			b.WriteString(`<a action="bypass -h Quest StatueOfShilen ` + e + `">` + e + "</a>")
		}
		b.WriteString("</body></html>")
		return b.String()
	}
	pages := map[string]string{"default/32109.htm": page("32109", []string{"enter"})}
	for name, events := range links {
		pages["script/ai/boss/sailren/StatueOfShilen/"+name] = page(name, events)
	}
	return pages
}

// sailrenSkills are the dummy's two production skills, as fixture
// definitions it can always pay for.
func sailrenSkills() *modelskill.Table {
	var defs []modelskill.Definition
	for _, id := range []modelskill.ID{5090, 5091} {
		defs = append(defs, modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
		})
	}
	return modelskill.NewTable(defs)
}

// sailrenNest is the nest's grand boss zone, around the dummy's territory.
func sailrenNest(t *testing.T) *zone.Boss {
	t.Helper()
	form, err := zone.NewCuboid(25700, 29400, -8700, -4700, -2560, 440)
	if err != nil {
		t.Fatal(err)
	}
	return zone.NewBoss(sailrenNestZone, form)
}

// bootSailren boots Sailren's island with the character entering the world
// at at holding gazkhs Gazkhs, every script draw 0, and the statue and the
// dummy as scripts.xml lists them. opts are added last.
func bootSailren(t *testing.T, at location.Location, gazkhs int32, opts ...gameservertest.Option) *sailrenWorld {
	t.Helper()
	w := &sailrenWorld{baiumWorld: &baiumWorld{start: time.Now()}, log: &sailrenLog{}}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sailren.xml"), []byte(sailrenSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	makers := script.MakerCatalog{
		"statue_of_shilen_maker": maker.StatueOfShilen,
		"sailren_dummy_maker":    maker.SailrenDummy,
		"sailren_probe": func() script.Maker {
			return script.Maker{MakerHooks: script.MakerHooks{
				OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
					w.log.add(fmt.Sprintf("maker %s %s", e.Group.Maker().Name, e.Name))
				},
			}}
		},
	}
	kinds := map[int32]script.NPCKind{
		sailrenStatueID: script.KindFolk, sailrenDummyID: script.KindFolk, sailrenCubeID: script.KindFolk, sailrenListenerID: script.KindFolk,
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.sailren.SailrenDummy"},
		{Path: "script.ai.boss.sailren.StatueOfShilen"},
		{Path: "test.SailrenListener"},
		{Path: "test." + sailrenDirector},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":       defaultnpc.New,
		"script.ai.boss.sailren.SailrenDummy":   sailren.SailrenDummy,
		"script.ai.boss.sailren.StatueOfShilen": sailren.StatueOfShilen,
		"test.SailrenListener": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{sailrenListenerID}, Hooks: script.Hooks{
				OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
					w.log.add(fmt.Sprintf("sailren heard %d %d %d", e.EventID, e.Arg1, e.Arg2))
				},
			}}
		},
		"test." + sailrenDirector: func() script.Script { return script.Script{} },
	}
	gazkh := &item.Template{
		ID: sailrenGazkh, Name: "Gazkh", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{at}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", int(sailrenStatueID)), gameservertest.FolkTemplate("Folk", int(sailrenDummyID)),
			gameservertest.FolkTemplate("Folk", int(sailrenCubeID)), gameservertest.FolkTemplate("Folk", int(sailrenListenerID)),
		})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithMakers(makers),
		gameservertest.WithNpcSpawnSkills(sailrenSkills()),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
		gameservertest.WithHTMLPages(sailrenPages()),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), gazkh))),
	}, opts...)...)
	if gazkhs > 0 {
		srv.GiveItem(t, srv.SoleObjectID(t), sailrenGazkh, gazkhs)
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
	return w
}

// sailrenMate is a second character in the world, in the party the
// character leads.
type sailrenMate struct {
	c  *testsupport.ScriptedClient
	id int32
}

// joinMate brings Mate into the world, into a party the character leads,
// and places it at at.
func (w *sailrenWorld) joinMate(t *testing.T, at location.Location) sailrenMate {
	t.Helper()
	m := sailrenMate{id: w.srv.SeedCharacterFor(t, "mate", "Mate", 20, 0).ObjectID()}
	m.c = w.srv.DialClient(t, "mate", 1)
	m.c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected for Mate within 100 frames")
		}
		if m.c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	m.c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	w.srv.ReadQueued(t, m.c)
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Client.Send(encodeJoinParty("Mate"))
	w.srv.ReadQueued(t, m.c)
	m.c.Send(encodeAnswerJoinParty(1))
	w.srv.ReadQueued(t, m.c)
	w.srv.ReadQueued(t, w.srv.Client)
	inParty := make(chan bool, 1)
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() { inParty <- script.PlayerOf(w.player).InParty() }) {
		t.Fatal("the character's queue is closed")
	}
	if !<-inParty {
		t.Fatal("the character is in no party after Mate accepted")
	}
	w.place(t, m.id, m.c, at)
	return m
}

// place teleports the online player id to at and lets its client appear
// there.
func (w *sailrenWorld) place(t *testing.T, id int32, c *testsupport.ScriptedClient, at location.Location) {
	t.Helper()
	w.srv.RunScript(t, id, sailrenDirector, func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0)
	})
	w.srv.Settle(t)
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, w.srv.Client)
	if x, y, _ := w.srv.PlayerPosition(t, id); x != at.X || y != at.Y {
		t.Fatalf("setup: player %d stands at %d,%d, want %d,%d", id, x, y, at.X, at.Y)
	}
}

// makerEvent runs maker script event name on the npcmaker named maker.
func (w *sailrenWorld) makerEvent(t *testing.T, maker, name string) {
	t.Helper()
	if !w.srv.NpcSpawns.MakerEvent(maker, name, 0, 0) {
		t.Fatalf("npcmaker %s not found by name", maker)
	}
	w.srv.Settle(t)
}

// scriptEvent sends the one live NPC of id script event eventID, from the
// director on the character's queue.
func (w *sailrenWorld) scriptEvent(t *testing.T, id, eventID int32) {
	t.Helper()
	n := soleBaiumNPC(t, w.baiumWorld, id)
	if !w.srv.RunScript(t, w.player.ObjectID(), sailrenDirector, func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(n), eventID, 0, 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// enter selects the statue, talks to it and asks to enter through c, and
// returns the names of the pages it answered with after its first page.
func (w *sailrenWorld) enter(t *testing.T, c *testsupport.ScriptedClient, event string) []string {
	t.Helper()
	statue := soleBaiumNPC(t, w.baiumWorld, sailrenStatueID)
	for range 2 {
		c.Send(encodeFolkAction(statue.ObjectID(), w.at))
		w.srv.Settle(t)
	}
	if got := baiumPageNames(w.srv.ReadQueued(t, c)); len(got) == 0 || slices.ContainsFunc(got, func(p string) bool { return p != "32109" }) {
		t.Fatalf("the statue's first pages = %q, want its own", got)
	}
	c.Send(encodeManagerBypass("Quest StatueOfShilen " + event))
	w.srv.Settle(t)
	return baiumPageNames(w.srv.ReadQueued(t, c))
}

// wantPage fails unless asking event of the statue through c answers page.
func (w *sailrenWorld) wantPage(t *testing.T, c *testsupport.ScriptedClient, event string, page ...string) {
	t.Helper()
	if got := w.enter(t, c, event); !slices.Equal(got, page) {
		t.Fatalf("%s: pages = %q, want %q", event, got, page)
	}
}

// bypass asks event of the statue through c, from the page the statue
// last showed, and returns the names of the pages it answered with.
func (w *sailrenWorld) bypass(t *testing.T, c *testsupport.ScriptedClient, event string) []string {
	t.Helper()
	c.Send(encodeManagerBypass("Quest StatueOfShilen " + event))
	w.srv.Settle(t)
	return baiumPageNames(w.srv.ReadQueued(t, c))
}

// second lets a second pass, runs the AI cycle that ends it and returns
// the frames sent meanwhile.
func (w *sailrenWorld) second(t *testing.T) [][]byte {
	t.Helper()
	frames := w.advance(t, time.Second)
	if err := w.srv.AI.Tick(); err != nil {
		t.Fatalf("AI tick: %v", err)
	}
	w.srv.Settle(t)
	return append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// sailrenFrames renders a step's frames as antharasFrames does, plus the
// production casts, in order.
func sailrenFrames(t *testing.T, frames [][]byte, roles map[int32]string) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeMagicSkillUse {
			out = append(out, antharasFrames(t, [][]byte{f}, roles)...)
			continue
		}
		v := func(i int) int32 { return int32(binary.LittleEndian.Uint32(f[1+4*i:])) }
		out = append(out, fmt.Sprintf("S MagicSkillUse caster=%s target=%s skill=%d", roles[v(0)], roles[v(1)], v(2)))
	}
	return out
}

// TestStatueOfShilenTurnsAwayTheUnready ports the statue's refusals and
// story (phase 1): a player in no party is told so (002); with a party,
// the nest closed until Sailren waits there (005); once open (event 11041
// through its maker), a member who does not lead is turned away (004), a
// leader without Gazkh is told to get one (003), and with a member over
// 1000 away the party is told to gather (party_range) and keeps its Gazkh.
// Event 11047 marks the nest busy (006) and 11050 frees it; 11045 closes
// it again. The story pages answer whatever the state.
func TestStatueOfShilenTurnsAwayTheUnready(t *testing.T) {
	w := bootSailren(t, sailrenAtStatue, 0)
	c := w.srv.Client
	w.wantPage(t, c, "enter", "statue_of_shilen002.htm")
	for _, step := range [][2]string{
		{"battle_prep", "statue_of_shilen002a.htm"}, {"enter_details", "statue_of_shilen003a.htm"}, {"learn_more", "statue_of_shilen004a.htm"},
	} {
		if got := w.bypass(t, c, step[0]); !slices.Equal(got, []string{step[1]}) {
			t.Fatalf("%s: pages = %q, want %q", step[0], got, step[1])
		}
	}

	mate := w.joinMate(t, w.at)
	w.wantPage(t, c, "enter", "statue_of_shilen005.htm")

	w.makerEvent(t, "rune16_npc2017_01m1", "11041")
	w.wantPage(t, mate.c, "enter", "statue_of_shilen004.htm")
	w.wantPage(t, c, "enter", "statue_of_shilen003.htm")

	w.srv.RunScript(t, w.player.ObjectID(), sailrenDirector, func(s *script.Script, p *script.Player) { s.GiveItems(p, sailrenGazkh, 1) })
	w.srv.Settle(t)
	w.srv.ReadQueued(t, c)
	w.place(t, mate.id, mate.c, location.Location{X: w.at.X - 1100, Y: w.at.Y, Z: w.at.Z})
	w.wantPage(t, c, "enter", "party_range.htm")
	if got := w.srv.PlayerItemCount(t, w.player.ObjectID(), sailrenGazkh); got != 1 {
		t.Fatalf("Gazkh after party_range = %d, want 1", got)
	}

	w.makerEvent(t, "rune16_npc2017_01m1", "11047")
	w.wantPage(t, c, "enter", "statue_of_shilen006.htm")
	w.makerEvent(t, "rune16_npc2017_01m1", "11050")
	w.wantPage(t, c, "enter", "party_range.htm")
	w.makerEvent(t, "rune16_npc2017_01m1", "11045")
	w.wantPage(t, c, "enter", "statue_of_shilen005.htm")
	w.log.expect(t, "the refusals")
}

// TestStatueOfShilenLetsThePartyIn ports the entry (phases 1 and 4): the
// leader of a party gathered within 1000, holding Gazkh, gives one up;
// each member may enter the nest's zone and is teleported to the nest's
// entry, and the statue runs 11053 on the three dinosaur makers, then
// 11048 on the dummy's, whose players are sent out: none stands in its
// territory yet. The nest is then busy (006), and a minute later the
// statue runs 1001 on the first dinosaur maker.
func TestStatueOfShilenLetsThePartyIn(t *testing.T) {
	nest := sailrenNest(t)
	zones := zone.NewIndex()
	zones.Add(nest)
	w := bootSailren(t, sailrenAtStatue, 1, gameservertest.WithZones(zones))
	mate := w.joinMate(t, location.Location{X: w.at.X - 900, Y: w.at.Y, Z: w.at.Z})
	w.makerEvent(t, "rune16_npc2017_01m1", "11041")

	statue := soleBaiumNPC(t, w.baiumWorld, sailrenStatueID)
	for range 2 {
		w.srv.Client.Send(encodeFolkAction(statue.ObjectID(), w.at))
		w.srv.Settle(t)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.ReadQueued(t, mate.c)
	w.srv.Client.Send(encodeManagerBypass("Quest StatueOfShilen enter"))
	w.srv.Settle(t)
	roles := map[int32]string{w.player.ObjectID(): "player", mate.id: "mate"}
	wantAntharasFrames(t, "enter", w.srv.ReadQueued(t, w.srv.Client), roles,
		"S TeleportToLocation obj=player loc=27549,-6638,-2008 fast=0",
	)
	wantAntharasFrames(t, "enter, Mate", w.srv.ReadQueued(t, mate.c), roles,
		"S TeleportToLocation obj=player loc=27549,-6638,-2008 fast=0",
		"S TeleportToLocation obj=mate loc=27549,-6638,-2008 fast=0",
	)
	w.log.expect(t, "enter",
		"maker rune20_mb2017_01m1 11053", "maker rune20_mb2017_02m1 11053", "maker rune20_mb2017_03m1 11053",
	)
	if got := w.srv.PlayerItemCount(t, w.player.ObjectID(), sailrenGazkh); got != 0 {
		t.Fatalf("Gazkh after the entry = %d, want 0", got)
	}
	allowed := nest.AllowedPlayers()
	slices.Sort(allowed)
	want := []int32{w.player.ObjectID(), mate.id}
	slices.Sort(want)
	if !slices.Equal(allowed, want) {
		t.Fatalf("players the nest admits = %v, want %v", allowed, want)
	}
	for _, c := range []*testsupport.ScriptedClient{w.srv.Client, mate.c} {
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
		w.srv.Settle(t)
	}
	for _, id := range want {
		if x, y, z := w.srv.PlayerPosition(t, id); x != 27549 || y != -6638 || z != -2008 {
			t.Fatalf("player %d stands at %d,%d,%d after the entry, want the nest's entry", id, x, y, z)
		}
	}
	w.srv.ReadQueued(t, mate.c)
	w.srv.ReadQueued(t, w.srv.Client)

	if busy := script.NPCOf(statue).Int(script.SlotIntAI1); busy != 1 {
		t.Fatalf("the statue's busy mark after the entry = %d, want 1", busy)
	}

	w.advance(t, 59*time.Second)
	w.log.expect(t, "59 s after the entry")
	w.advance(t, time.Second)
	w.log.expect(t, "a minute after the entry", "maker rune20_mb2017_01m1 1001")
}

// TestSailrenDummyOustsTheNest ports the dummy's script events (phase 2):
// 11046, 11047 and 11048 through its maker each send the players in its
// territory to the nest's first exit (every draw 0); 11051 does too, then
// runs 11052 and 1000 on each dinosaur maker and 11050 on the statue's
// maker, which frees the nest. 11054 leaves the teleport cube.
func TestSailrenDummyOustsTheNest(t *testing.T) {
	w := bootSailren(t, sailrenInNest, 0)
	roles := map[int32]string{w.player.ObjectID(): "player"}
	for _, name := range []string{"11046", "11047", "11048"} {
		w.makerEvent(t, "rune16_npc2017_13m1", name)
		frames := w.srv.ReadQueued(t, w.srv.Client)
		got := sailrenFrames(t, frames, roles)
		if len(got) != 1 {
			t.Fatalf("%s frames = %q, want one teleport", name, got)
		}
		w.appear(t)
		x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
		if x < 23475 || x > 23675 || y < -7827 || y > -7627 {
			t.Fatalf("%s: the character stands at %d,%d, want within 100 of 23575,-7727", name, x, y)
		}
		w.place(t, w.player.ObjectID(), w.srv.Client, sailrenInNest)
	}

	w.makerEvent(t, "rune16_npc2017_01m1", "11041")
	w.makerEvent(t, "rune16_npc2017_01m1", "11047")
	w.scriptEvent(t, sailrenDummyID, 11051)
	if got := sailrenFrames(t, w.srv.ReadQueued(t, w.srv.Client), roles); len(got) != 1 {
		t.Fatalf("11051 frames = %q, want one teleport", got)
	}
	w.appear(t)
	w.log.expect(t, "11051",
		"maker rune20_mb2017_01m1 11052", "maker rune20_mb2017_01m1 1000",
		"maker rune20_mb2017_02m1 11052", "maker rune20_mb2017_02m1 1000",
		"maker rune20_mb2017_03m1 11052", "maker rune20_mb2017_03m1 1000",
	)
	statue := script.NPCOf(soleBaiumNPC(t, w.baiumWorld, sailrenStatueID))
	if busy := statue.Int(script.SlotIntAI1); busy != 0 {
		t.Fatalf("the statue's busy mark after 11051 = %d, want 0", busy)
	}

	if n := len(baiumNPCs(w.baiumWorld, sailrenCubeID)); n != 0 {
		t.Fatalf("%d cubes before 11054, want none", n)
	}
	w.scriptEvent(t, sailrenDummyID, 11054)
	cube := soleBaiumNPC(t, w.baiumWorld, sailrenCubeID)
	if got, want := placeOf(cube), "27644,-6638,-2008 heading 0"; got != want {
		t.Fatalf("the cube stands at %s, want %s", got, want)
	}
	if master := script.NPCOf(cube).Master(); master == nil || master.ObjectID() != soleBaiumNPC(t, w.baiumWorld, sailrenDummyID).ObjectID() {
		t.Fatal("the cube is no private of the dummy")
	}
}

// TestSailrenDummyPlaysTheCutscene ports the dummy's cutscene (phase 3):
// seeing a player in a party it keeps the leader. Event 11044 starts it
// 180006 ms later: the first shot and six production casts, five more
// shots 4.1, 7.1, 10.1, 13.1 and 16.1 s in and the last at 23.1 s, the
// casts renewed on timers 1001 to 1006, and 15 s after the last shot it
// tells Sailren, within 1000, event 11055 with the leader.
func TestSailrenDummyPlaysTheCutscene(t *testing.T) {
	w := bootSailren(t, sailrenAtStatue, 0)
	w.joinMate(t, sailrenInNest)
	w.place(t, w.player.ObjectID(), w.srv.Client, sailrenInNest)
	if err := w.srv.AI.Tick(); err != nil {
		t.Fatalf("AI tick: %v", err)
	}
	w.srv.Settle(t)
	dummy := soleBaiumNPC(t, w.baiumWorld, sailrenDummyID)
	if got := script.NPCOf(dummy).Int(script.SlotIntAI0); got != w.player.ObjectID() {
		t.Fatalf("the leader the dummy keeps = %d, want the character %d", got, w.player.ObjectID())
	}

	roles := map[int32]string{w.player.ObjectID(): "player", dummy.ObjectID(): "dummy"}
	w.scriptEvent(t, sailrenDummyID, 11044)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := sailrenFrames(t, w.advance(t, 180005*time.Millisecond), roles); len(got) != 0 {
		t.Fatalf("frames before the cutscene = %q, want none", got)
	}
	var got []string
	for i := range 40 {
		for _, line := range sailrenFrames(t, w.second(t), roles) {
			got = append(got, fmt.Sprintf("%d %s", i+1, line))
		}
	}
	cast := func(sec, id int) string {
		return fmt.Sprintf("%d S MagicSkillUse caster=dummy target=dummy skill=%d", sec, id)
	}
	camera := func(sec int, shot string) string {
		return fmt.Sprintf("%d S SpecialCamera obj=dummy shot=%s", sec, shot)
	}
	want := []string{
		// Timer 3001: the first shot, and the two production desires,
		// each cast in turn.
		camera(1, "60,110,30,4000,20000,0,65,1,0"), cast(1, 5090), cast(2, 5091),
		cast(4, 5090), // timer 1001, 3.4 s in
		camera(5, "100,180,30,3000,20000,0,50,1,0"), // timer 2000, 4.1 s in
		cast(7, 5090), // timer 1002, 6.8 s in
		camera(8, "150,270,25,3000,20000,0,30,1,0"),                   // timer 2001, 7.1 s in
		camera(11, "160,360,20,3000,20000,10,15,1,0"), cast(11, 5090), // timers 2002 and 1003
		camera(14, "160,450,10,3000,20000,0,10,1,0"), cast(14, 5090), // timers 2003 and 1004
		camera(17, "160,560,0,3000,20000,0,10,1,0"), cast(17, 5091), // timers 2004 and 1005
		camera(24, "70,560,0,500,7000,-15,20,1,0"), cast(24, 5091), // timers 2005 and 1006
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the cutscene:\n got %q\nwant %q", got, want)
	}
	w.log.expect(t, "the cutscene", fmt.Sprintf("sailren heard 11055 %d 0", w.player.ObjectID()))
}
