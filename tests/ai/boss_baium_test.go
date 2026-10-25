package ai

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/baium"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
	"github.com/rs/zerolog"
)

// Baium, its archangels, its stone, the cube it leaves, the Angelic Vortex
// up to its lair and the Blooded Fabric the vortex takes.
const (
	baiumID          = int32(29020)
	baiumArchangelID = int32(29021)
	baiumStoneID     = int32(29025)
	baiumCubeID      = int32(31842)
	baiumVortexID    = int32(31862)
	bloodedFabric    = int32(4295)
)

// baiumHidden is where Baium's spawn places it, outside its lair; baiumLair
// is where its stone stands and it wakes; baiumCharacter is where the
// character enters the world, inside the lair and in reach of the stone.
var (
	baiumHidden    = location.Location{X: -105200, Y: -253104, Z: -15264}
	baiumLair      = location.Location{X: 116033, Y: 17447, Z: 10107}
	baiumCharacter = location.Location{X: 115960, Y: 17400, Z: 10107}
)

// baiumSpawnlist is the shipped lair territory and Baium's npcmaker, with
// Baium spawning at pos, and the Angelic Vortex beside the character.
const baiumSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="23_18_baium_npc" minZ="10036" maxZ="10836">
		<node x="117660" y="15328"/>
		<node x="117508" y="16916"/>
		<node x="116688" y="18088"/>
		<node x="115180" y="19080"/>
		<node x="112364" y="17856"/>
		<node x="111680" y="15524"/>
		<node x="113820" y="13252"/>
		<node x="116484" y="13856"/>
	</territory>
	<npcmaker name="23_18_baium_npc" territory="23_18_baium_npc" maximumNpcs="10">
		<ai type="default_maker"/>
		<npc id="29020" pos="%s" total="1" respawn="168hour" respawnRand="48hour" dbName="baium" dbSaving="DEATH_TIME;PARAMETERS;POS"/>
	</npcmaker>
	<npcmaker name="baium_vortex" territory="23_18_baium_npc" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="31862" pos="115940;17420;10107;0" total="1" respawn="60sec"/>
	</npcmaker>
</list>`

// baiumPages are the pages the stone and the vortex answer, each naming
// itself, with the links of the shipped pages: the stone's wake and the
// vortex's entry.
func baiumPages() map[string]string {
	pages := map[string]string{
		"default/29025.htm": `<html><body>29025<a action="bypass -h npc_%objectId%_Quest BaiumStone">Wake Baium</a></body></html>`,
	}
	for i := range 6 {
		name := fmt.Sprintf("31862-%02d.htm", i)
		pages["script/ai/boss/baium/BaiumAngelicVortex/"+name] = "<html><body>" + name + "</body></html>"
	}
	pages["script/ai/boss/baium/BaiumAngelicVortex/31862-00.htm"] = `<html><body>31862-00.htm<a action="bypass -h Quest BaiumAngelicVortex enter">Go</a></body></html>`
	return pages
}

// baiumItems is the fixture catalog with the Blooded Fabric.
func baiumItems() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: bloodedFabric, Name: "Blooded Fabric", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	}))
}

// baiumSkills are the skills Baium casts, as fixture definitions it can
// always pay for.
func baiumSkills() *modelskill.Table {
	var defs []modelskill.Definition
	for _, id := range []modelskill.ID{4127, 4128, 4129, 4130, 4131, 4132, 4133, 4135, 4136, 4258} {
		defs = append(defs, modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		})
	}
	return modelskill.NewTable(defs)
}

// baiumWorld is the character in Baium's lair, with the wall clock the
// in-game clock reads moved along with the server's.
type baiumWorld struct {
	*warriorWorld
	start   time.Time
	elapsed atomic.Int64
}

// now is the in-game clock's wall clock: the boot's time plus what the
// world advanced since.
func (w *baiumWorld) now() time.Time { return w.start.Add(time.Duration(w.elapsed.Load())) }

// bootBaium boots Baium's lair with Baium spawning at pos, backed by the
// spawn_data table, every script draw 0, the Baium scripts as scripts.xml
// lists them, and the character entering the world inside the lair. opts
// are added last.
func bootBaium(t *testing.T, pos string, opts ...gameservertest.Option) *baiumWorld {
	t.Helper()
	return bootBaiumHolding(t, pos, 0, opts...)
}

// bootBaiumHolding is bootBaium with the character holding fabrics Blooded
// Fabrics as it enters the world.
func bootBaiumHolding(t *testing.T, pos string, fabrics int32, opts ...gameservertest.Option) *baiumWorld {
	t.Helper()
	w := &baiumWorld{start: time.Now()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "baium.xml"), []byte(fmt.Sprintf(baiumSpawnlist, pos)), 0o644); err != nil {
		t.Fatal(err)
	}
	spawns, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	boss := warriorTemplate(baiumID, nil)
	boss.Type, boss.Name, boss.Level, boss.HPMax = "GrandBoss", "Baium", 75, 1000000
	boss.AggroRange, boss.CanSeeThrough = 1000, true
	angel := warriorTemplate(baiumArchangelID, nil)
	angel.Name = "Archangel"
	kinds := map[int32]script.NPCKind{
		baiumID: script.KindHostile, baiumArchangelID: script.KindHostile, baiumStoneID: script.KindFolk, baiumCubeID: script.KindFolk,
		baiumVortexID: script.KindFolk,
	}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.baium.Archangel"},
		{Path: "script.ai.boss.baium.Baium"},
		{Path: "script.ai.boss.baium.BaiumAngelicVortex"},
		{Path: "script.ai.boss.baium.BaiumStone"},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":         defaultnpc.New,
		"script.ai.boss.baium.Archangel":          baium.Archangel,
		"script.ai.boss.baium.Baium":              baium.Baium,
		"script.ai.boss.baium.BaiumAngelicVortex": baium.BaiumAngelicVortex,
		"script.ai.boss.baium.BaiumStone":         baium.BaiumStone,
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{baiumCharacter}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			boss, angel, gameservertest.FolkTemplate("Folk", int(baiumStoneID)), gameservertest.FolkTemplate("Folk", int(baiumCubeID)),
			gameservertest.FolkTemplate("Folk", int(baiumVortexID)),
		})),
		gameservertest.WithNpcSpawns(spawns),
		gameservertest.WithSpawnData(),
		gameservertest.WithNpcSpawnSkills(baiumSkills()),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithGameClockNow(w.now),
		gameservertest.WithHTMLPages(baiumPages()),
		gameservertest.WithItemTemplates(baiumItems()),
	}, opts...)...)
	if fabrics > 0 {
		srv.GiveItem(t, srv.SoleObjectID(t), bloodedFabric, fabrics)
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

// baiumNPCs returns the live NPCs of id, hostile or civilian.
func baiumNPCs(w *baiumWorld, id int32) []attackable.Combatant {
	var out []attackable.Combatant
	for _, obj := range w.srv.State.Objects() {
		switch o := obj.(type) {
		case *npc.Hostile:
			if int32(o.Instance.Template.ID) == id && !o.Decayed() && !o.Dead() {
				out = append(out, o)
			}
		case *npc.Folk:
			if int32(o.Instance.Template.ID) == id && !o.Decayed() {
				out = append(out, o)
			}
		}
	}
	return out
}

// soleBaiumNPC returns the one live NPC of id.
func soleBaiumNPC(t *testing.T, w *baiumWorld, id int32) attackable.Combatant {
	t.Helper()
	found := baiumNPCs(w, id)
	if len(found) != 1 {
		t.Fatalf("%d live NPCs of %d, want 1", len(found), id)
	}
	return found[0]
}

// placeOf renders where c stands and faces as "x,y,z heading h".
func placeOf(c attackable.Combatant) string {
	x, y, z := c.Position()
	return fmt.Sprintf("%d,%d,%d heading %d", x, y, z, c.Heading())
}

// baiumFrames renders the frames of a Baium step the tests compare, in
// order: sounds, NPC lines, teleports, screen messages, social actions and
// earthquakes. roles names object ids.
func baiumFrames(t *testing.T, frames [][]byte, roles map[int32]string) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case 0x98, serverpackets.OpcodeNpcSay, serverpackets.OpcodeTeleportToLocation:
			line, err := scriptcontract.Packet(f, roles)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, line)
		case serverpackets.OpcodeSocialAction:
			obj := int32(binary.LittleEndian.Uint32(f[1:5]))
			out = append(out, fmt.Sprintf("S SocialAction obj=%s action=%d", roles[obj], int32(binary.LittleEndian.Uint32(f[5:9]))))
		case serverpackets.OpcodeEarthquake:
			v := func(i int) int32 { return int32(binary.LittleEndian.Uint32(f[1+4*i:])) }
			out = append(out, fmt.Sprintf("S Earthquake loc=%d,%d,%d intensity=%d duration=%d npc=%d", v(0), v(1), v(2), v(3), v(4), v(5)))
		}
	}
	for _, text := range screenTexts(frames) {
		out = append(out, "S ExShowScreenMessage "+text)
	}
	return out
}

// wantBaiumFrames fails unless the step's rendered frames are want.
func wantBaiumFrames(t *testing.T, step string, frames [][]byte, roles map[int32]string, want ...string) {
	t.Helper()
	if got := baiumFrames(t, frames, roles); !slices.Equal(got, want) {
		t.Fatalf("%s frames:\n got %q\nwant %q", step, got, want)
	}
}

// talkTo selects f, then talks to it, and returns the frames sent.
func (w *baiumWorld) talkTo(t *testing.T, f attackable.Combatant) [][]byte {
	t.Helper()
	for range 2 {
		w.srv.Client.Send(encodeFolkAction(f.ObjectID(), w.at))
		w.srv.Settle(t)
	}
	return w.srv.ReadQueued(t, w.srv.Client)
}

// appear tells the server the client appeared where its teleport sent it,
// as a real client does.
func (w *baiumWorld) appear(t *testing.T) {
	t.Helper()
	w.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// advance lets d pass and returns the frames sent meanwhile.
func (w *baiumWorld) advance(t *testing.T, d time.Duration) [][]byte {
	t.Helper()
	w.elapsed.Add(int64(d))
	w.srv.Advance(t, d)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// baiumDBValue shuts the server down and returns the db value saved with
// Baium's spawn_data row.
func baiumDBValue(t *testing.T, w *baiumWorld) int {
	t.Helper()
	w.srv.Shutdown(t)
	states, err := gamesql.NewSpawnStore(w.srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state, ok := states["baium"]
	if !ok {
		t.Fatal("no spawn_data row baium")
	}
	return state.DBValue
}

// wantArchangels fails unless Baium's five archangels stand where it
// calls them, each with its index as its first spawn parameter.
func wantArchangels(t *testing.T, w *baiumWorld) {
	t.Helper()
	var got []string
	for _, c := range baiumNPCs(w, baiumArchangelID) {
		got = append(got, fmt.Sprintf("%s p1 %d", placeOf(c), script.NPCOf(c).Int(script.SlotParam1)))
	}
	slices.Sort(got)
	want := []string{
		"114239,17168,10136 heading -1992 p1 5",
		"114880,16236,10136 heading 5400 p1 4",
		"115168,17200,10136 heading 0 p1 2",
		"115780,15564,10136 heading 13620 p1 3",
		"115792,16608,10136 heading 0 p1 1",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("archangels:\n got %q\nwant %q", got, want)
	}
}

// talkToStone opens the stone's page and asks it to wake Baium, and
// returns the frames of the wake.
func (w *baiumWorld) talkToStone(t *testing.T, stone attackable.Combatant) [][]byte {
	t.Helper()
	if got := baiumPageNames(w.talkTo(t, stone)); len(got) == 0 || slices.ContainsFunc(got, func(p string) bool { return p != "29025" }) {
		t.Fatalf("the stone's pages = %q, want its own", got)
	}
	w.srv.Client.Send(encodeManagerBypass(fmt.Sprintf("npc_%d_Quest BaiumStone", stone.ObjectID())))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// baiumPageNames returns the page each NpcHtmlMessage among frames shows,
// by the name the fixture page carries.
func baiumPageNames(frames [][]byte) []string {
	var out []string
	for _, text := range managerHTML(frames) {
		text = strings.TrimPrefix(text, "<html><body>")
		name, _, _ := strings.Cut(text, "<")
		out = append(out, name)
	}
	return out
}

// wakeBaium talks to the stone, which wakes Baium in its lair, and returns
// Baium.
func wakeBaium(t *testing.T, w *baiumWorld) attackable.Combatant {
	t.Helper()
	boss := soleBaiumNPC(t, w, baiumID)
	w.talkToStone(t, soleBaiumNPC(t, w, baiumStoneID))
	if n := len(baiumNPCs(w, baiumStoneID)); n != 0 {
		t.Fatalf("%d stones after the wake, want none", n)
	}
	return boss
}

// TestBaiumSleepsAndWakes ports Baium's sleep, wake and call (phases 1 and
// 2): spawned outside its lair it names itself under global memo key 2,
// stores db value 0 and leaves its stone in the lair, facing -25348. The
// stone's script event turns it to -25348, teleports it into the lair
// and stores db value 1; 2 s later it shakes the screens around (40 for
// 10 s) and plays BS02_A, at 11.5 s it plays social action 3, 7.3 s later
// social action 1: it says its line 22937, teleports the waker beside it
// and wants to cast 4136 at it with 1100000 (no hero is around for its
// screen message). 8 s later its five archangels appear, and the db value
// 1 is saved.
func TestBaiumSleepsAndWakes(t *testing.T) {
	w := bootBaium(t, "-105200;-253104;-15264;0")
	boss := soleBaiumNPC(t, w, baiumID)
	if got, want := placeOf(boss), fmt.Sprintf("%d,%d,%d heading 0", baiumHidden.X, baiumHidden.Y, baiumHidden.Z); got != want {
		t.Fatalf("Baium stands at %s, want %s", got, want)
	}
	if got, want := placeOf(soleBaiumNPC(t, w, baiumStoneID)), "116033,17447,10107 heading -25348"; got != want {
		t.Fatalf("the stone stands at %s, want %s", got, want)
	}
	if n := len(baiumNPCs(w, baiumArchangelID)); n != 0 {
		t.Fatalf("%d archangels before the wake, want none", n)
	}

	wakeBaium(t, w)
	if got, want := placeOf(boss), fmt.Sprintf("%d,%d,%d heading -25348", baiumLair.X, baiumLair.Y, baiumLair.Z); got != want {
		t.Fatalf("woken Baium stands at %s, want %s", got, want)
	}
	roles := map[int32]string{boss.ObjectID(): "baium", w.player.ObjectID(): "player"}

	wantBaiumFrames(t, "2 s", w.advance(t, 2*time.Second), roles,
		"S Earthquake loc=116033,17447,10107 intensity=40 duration=10 npc=1",
		"S PlaySound type=1 file=BS02_A bind=1 obj=baium loc=116033,17447,10107 delay=0")
	wantBaiumFrames(t, "11.5 s", w.advance(t, 9500*time.Millisecond), roles, "S SocialAction obj=baium action=3")
	line, _ := npcstring.Text(22937)
	wantBaiumFrames(t, "18.8 s", w.advance(t, 7300*time.Millisecond), roles,
		"S SocialAction obj=baium action=1",
		fmt.Sprintf("S NpcSay obj=baium type=0 npc=%d text=%s", 1000000+baiumID, scriptcontract.Quote(line)),
		"S TeleportToLocation obj=player loc=115910,17337,10105 fast=0")
	if x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID()); x != 115910 || y != 17337 || z != 10105 {
		t.Fatalf("the waker stands at %d,%d,%d, want 115910,17337,10105", x, y, z)
	}
	w.appear(t)
	d, ok := desire(boss.(*npc.Hostile), actorai.IntentionCast)
	if !ok || d.Skill.ID != 4136 || d.Weight != 1100000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Baium's cast desire = %+v (%v), want 4136 at the waker with 1100000", d, ok)
	}

	if n := len(baiumNPCs(w, baiumArchangelID)); n != 0 {
		t.Fatalf("%d archangels before 26.8 s, want none", n)
	}
	w.advance(t, 8*time.Second)
	wantArchangels(t, w)

	if got := baiumDBValue(t, w); got != 1 {
		t.Fatalf("saved db value = %d, want 1", got)
	}
}

// TestBaiumSleepsAfterThirtyQuietMinutes ports Baium's reset (phase 4):
// its check every minute finds 30 minutes, and no more, since the wake at
// 30 minutes, and goes on; at 31 minutes it drops its desires, stores db
// value 0, sends the players of its lair to 120112,18208,-5152 (up to 900
// off on each axis), teleports back out of its lair and leaves a new stone
// at 116026,17435,10107, facing -25348. The db value 0 is saved.
func TestBaiumSleepsAfterThirtyQuietMinutes(t *testing.T) {
	w := bootBaium(t, "-105200;-253104;-15264;0")
	boss := wakeBaium(t, w)
	roles := map[int32]string{boss.ObjectID(): "baium", w.player.ObjectID(): "player"}
	w.advance(t, 18800*time.Millisecond)
	w.appear(t)
	w.advance(t, 30*time.Minute-18800*time.Millisecond)
	if x, y, z := boss.Position(); (location.Location{X: x, Y: y, Z: z}) != baiumLair {
		t.Fatalf("Baium at 30 minutes stands at %d,%d,%d, want its lair", x, y, z)
	}

	frames := w.advance(t, time.Minute)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if x < 120112-900 || x > 120112+900 || y < 18208-900 || y > 18208+900 || z != -5152 {
		t.Fatalf("the character stands at %d,%d,%d, want within 900 of 120112,18208,-5152", x, y, z)
	}
	// The character's teleport runs on its own queue once Baium's hook is
	// done (docs/script-engine-plan.md section 9, difference 4), so it
	// still sees Baium leave first.
	wantBaiumFrames(t, "31 minutes", frames, roles,
		"S TeleportToLocation obj=baium loc=-105200,-253104,-15264 fast=0",
		fmt.Sprintf("S TeleportToLocation obj=player loc=%d,%d,%d fast=0", x, y, z))
	if bx, by, bz := boss.Position(); (location.Location{X: bx, Y: by, Z: bz}) != baiumHidden {
		t.Fatalf("Baium stands at %d,%d,%d, want %v", bx, by, bz, baiumHidden)
	}
	if desires := boss.(*npc.Hostile).AI().Desires().Snapshot(); len(desires) != 0 {
		t.Fatalf("Baium's desires after the reset = %+v, want none", desires)
	}
	if got, want := placeOf(soleBaiumNPC(t, w, baiumStoneID)), "116026,17435,10107 heading -25348"; got != want {
		t.Fatalf("the new stone stands at %s, want %s", got, want)
	}

	if got := baiumDBValue(t, w); got != 0 {
		t.Fatalf("saved db value = %d, want 0", got)
	}
}

// TestBaiumResumesInsideItsLair ports Baium's restore (phase 1): spawned
// inside its lair, as a restart restores the fight's position, it leaves
// no stone, stores db value 1 and calls its five archangels 5 s later. The
// db value 1 is saved.
func TestBaiumResumesInsideItsLair(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	if n := len(baiumNPCs(w, baiumStoneID)); n != 0 {
		t.Fatalf("%d stones, want none", n)
	}
	w.advance(t, 4900*time.Millisecond)
	if n := len(baiumNPCs(w, baiumArchangelID)); n != 0 {
		t.Fatalf("%d archangels before 5 s, want none", n)
	}
	w.advance(t, 100*time.Millisecond)
	wantArchangels(t, w)

	if got := baiumDBValue(t, w); got != 1 {
		t.Fatalf("saved db value = %d, want 1", got)
	}
}

// TestBaiumRanksWhatItSees ports Baium's target ranking (phase 3), every
// draw 0: awake and with nothing to do, it wants to wander and looks
// around, and ranks the character it sees first with 1000. Wandering, a
// plain hit of 7 from the character counts toward its damage and raises
// the character's points to 7000; it then casts at its best target, the
// character: above 3/4 of its HP its first draw of 10 in 100 picks 4128,
// wanted with 1000000, and its draw of 70 in 100 drops the target's points
// to 500.
func TestBaiumRanksWhatItSees(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	handle := script.NPCOf(boss)

	for i := 0; boss.AI().CurrentIntention() != actorai.IntentionWander; i++ {
		if i == 10 {
			t.Fatalf("Baium's intention after 10 AI ticks = %v, want wander", boss.AI().CurrentIntention())
		}
		w.ticks(t, 1)
	}
	// Baium also ranks the archangels it sees, so the character's slot
	// among the three is the order of its looks.
	pts := -1
	for i, who := range []script.CreatureSlot{npc.CreatureQuest0, npc.CreatureQuest1, npc.CreatureQuest2} {
		if got := handle.Slot(who); got != nil && got.ObjectID() == w.player.ObjectID() {
			pts = i
		}
	}
	if pts < 0 {
		t.Fatal("Baium ranks no slot for the character")
	}
	ptsSlot := []script.IntSlot{npc.IntQuest0, npc.IntQuest1, npc.IntQuest2}[pts]
	if got := handle.Int(ptsSlot); got != 1000 {
		t.Fatalf("the character's points = %d, want 1000", got)
	}
	if _, ok := desire(boss, actorai.IntentionCast); ok {
		t.Fatal("Baium wants to cast before the hit")
	}

	w.hit(t, boss, 7)
	if got := handle.Int(script.SlotIntAI0); got != 1+7 {
		t.Fatalf("Baium's damage count = %d, want 8", got)
	}
	d, ok := desire(boss, actorai.IntentionCast)
	if !ok || d.Skill.ID != 4128 || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Baium's cast desire = %+v (%v), want 4128 at the character with 1000000", d, ok)
	}
	if got := handle.Int(ptsSlot); got != 500 {
		t.Fatalf("the character's points after the cast = %d, want 500", got)
	}
}

// TestBaiumDeathLeavesTheCube ports Baium's death (phase 5): dying, it
// leaves the teleport cube at 115017,15549,10090, plays BS01_D and clears
// global memo key 2, so the Angelic Vortex then finds no Baium: its entry
// answers 31862-04.htm.
func TestBaiumDeathLeavesTheCube(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	roles := map[int32]string{boss.ObjectID(): "baium", w.player.ObjectID(): "player"}

	w.onQueue(t, boss, func() { script.NPCOf(boss).DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	x, y, z := boss.Position()
	wantBaiumFrames(t, "dying hooks", w.advance(t, 3*time.Second), roles,
		fmt.Sprintf("S PlaySound type=1 file=BS01_D bind=1 obj=baium loc=%d,%d,%d delay=0", x, y, z))
	cube := soleBaiumNPC(t, w, baiumCubeID)
	if got, want := placeOf(cube), "115017,15549,10090 heading 0"; got != want {
		t.Fatalf("the cube stands at %s, want %s", got, want)
	}
	if got := baiumPageNames(w.enterVortex(t)); !slices.Equal(got, []string{"31862-04.htm"}) {
		t.Fatalf("the vortex's entry after the death = %q, want 31862-04.htm", got)
	}
}

// TestBaiumWeighsItsPartysDamage ports Baium's half of the fight's damage
// check (phase 3), every draw 0: resumed in its lair with its archangels
// out, a hit of 7 on an archangel adds 7 to its party's damage count; a
// hit of 5 on Baium itself adds 5 to its own damage count and nothing to
// its party's, nor does a hit of 4 from Baium on the archangel. Its check at 60.002 s scales the party's count 8 by 2.73 to
// 21, finds the gap to its own 6 over 3/10 of their sum, drops its flag to
// 0 and sets both counts back to 1.
func TestBaiumWeighsItsPartysDamage(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	angel := baiumNPCs(w, baiumArchangelID)[0].(*npc.Hostile)

	w.hit(t, angel, 7)
	if got := handle.Int(script.SlotIntAI1); got != 1+7 {
		t.Fatalf("Baium's party damage count after the archangel's hit = %d, want 8", got)
	}
	if got := handle.Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("Baium's own damage count after the archangel's hit = %d, want 1", got)
	}
	w.hit(t, boss, 5)
	if got := handle.Int(script.SlotIntAI0); got != 1+5 {
		t.Fatalf("Baium's own damage count after its hit = %d, want 6", got)
	}
	if got := handle.Int(script.SlotIntAI1); got != 8 {
		t.Fatalf("Baium's party damage count after its own hit = %d, want it kept at 8", got)
	}
	done := make(chan struct{})
	if !boss.Queue().Post(func() {
		defer close(done)
		angel.TakeDamage(4, boss)
	}) {
		t.Fatal("Baium's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	if got := handle.Int(script.SlotIntAI1); got != 8 {
		t.Fatalf("Baium's party damage count after its own hit on the archangel = %d, want it kept at 8", got)
	}
	if got := handle.Int(script.SlotFlag); got != 1 {
		t.Fatalf("Baium's flag before its check = %d, want 1", got)
	}

	w.advance(t, 55001*time.Millisecond)
	if got := handle.Int(script.SlotIntAI1); got != 8 {
		t.Fatalf("Baium's party damage count at 60.001 s = %d, want 8 before its check", got)
	}
	w.advance(t, time.Millisecond)
	if got := handle.Int(script.SlotFlag); got != 0 {
		t.Fatalf("Baium's flag after its check = %d, want 0", got)
	}
	if a0, a1 := handle.Int(script.SlotIntAI0), handle.Int(script.SlotIntAI1); a0 != 1 || a1 != 1 {
		t.Fatalf("Baium's damage counts after its check = %d and %d, want both 1", a0, a1)
	}
}
