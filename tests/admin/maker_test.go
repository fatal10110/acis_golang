package admin

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// makerAdminSpawnlist holds, around the fixture character at (10, 20):
// a triangle territory with an on-start maker and an event maker, a maker
// over two territories (merged as grouped_000), and, far away, a maker of
// a listed event and one more.
const makerAdminSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="admin_field" minZ="-1000" maxZ="1000"><node x="-1000" y="-1000"/><node x="1000" y="-1000"/><node x="0" y="1000"/></territory>
	<territory name="admin_west" minZ="-500" maxZ="500"><node x="-200" y="-200"/><node x="200" y="-200"/><node x="200" y="200"/><node x="-200" y="200"/></territory>
	<territory name="admin_far" minZ="-2000" maxZ="0"><node x="50000" y="50000"/><node x="51000" y="50000"/><node x="51000" y="51000"/></territory>
	<npcmaker name="admin_m1" territory="admin_field" maximumNpcs="2">
		<ai type="default_maker"/>
		<npc id="21101" total="2" pos="40;20;30;0" respawn="90sec" respawnRand="30sec"/>
	</npcmaker>
	<npcmaker name="admin_ev" territory="admin_field" event="admin_event" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21102" total="1" pos="60;20;30;0" respawn="1hour"/>
	</npcmaker>
	<npcmaker name="admin_grp" territory="admin_west;admin_far" maximumNpcs="1">
		<npc id="21103" total="1" pos="80;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="admin_age" territory="admin_far" event="18age" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21104" total="1" pos="50500;50200;-100;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="admin_away" territory="admin_far" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
	</npcmaker>
</list>`

func makerAdminTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "makers.xml"), []byte(makerAdminSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

func makerAdminTemplates() *npc.Table {
	var templates []*npc.Template
	for _, id := range []int{21101, 21102, 21103, 21104} {
		templates = append(templates, &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Name: "Maker Admin Wolf", Level: 1, HPMax: 100,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true,
		})
	}
	return npc.NewTable(templates)
}

// makerEvents records the maker script events the population is sent.
type makerEvents struct {
	mu   sync.Mutex
	seen []string
}

func (e *makerEvents) heard(maker, event string, _, _ int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = append(e.seen, maker+":"+event)
}

func (e *makerEvents) list() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.seen)
}

func bootMakerAdmin(t *testing.T, opts ...gameservertest.Option) *testsupport.ScriptedClient {
	t.Helper()
	opts = append([]gameservertest.Option{
		gameservertest.WithHTMLPages(shippedAdminPages(t, "spawnlist/maker_check.htm", "spawnlist/maker_info.htm", "spawnlist/maker_scripts.htm")),
		gameservertest.WithNPCs(makerAdminTemplates()),
		gameservertest.WithNpcSpawns(makerAdminTable(t)),
	}, opts...)
	srv, _ := bootAdmin(t, adminLevel, opts...)
	enterWorld(t, srv.Client)
	return srv.Client
}

// pageOf returns the one NpcHtmlMessage among frames.
func pageOf(t *testing.T, frames [][]byte) string {
	t.Helper()
	var pages []string
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNpcHtmlMessage {
			pages = append(pages, htmlBody(t, f))
		}
	}
	if len(pages) != 1 {
		t.Fatalf("frames = %x, want one NpcHtmlMessage", testsupport.FrameOpcodes(frames))
	}
	return pages[0]
}

func assertContains(t *testing.T, page string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(page, w) {
			t.Fatalf("page = %q, want it to contain %q", page, w)
		}
	}
}

func TestAdminMakerPages(t *testing.T) {
	events := &makerEvents{}
	c := bootMakerAdmin(t, gameservertest.WithMakerEventsHeard(events.heard))

	// The makers around the character, one territory a page: admin_field
	// with its two makers first, then the merged territory.
	page := pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker")))
	wantField := `<table><tr><td width=280><font color=LEVEL>admin_field</font></td></tr></table>` +
		`<table><tr><td width=170>min=-1000, max=1000</td><td width=110><a action="bypass -h admin_terr goto admin_field">Go To</a> | ` +
		`<a action="bypass -h admin_terr test admin_field">Test</a> | <a action="bypass -h admin_terr visual admin_field">Visualize</a></td></tr></table>` +
		`<table width=280 bgcolor=000000><tr><td width=10></td><td width=270><a action="bypass -h admin_maker admin_m1">admin_m1</a></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>` +
		`<table width=280><tr><td width=10></td><td width=270><a action="bypass -h admin_maker admin_ev">admin_ev</a></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>` +
		`<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center><img height=2><button action="bypass admin_maker 1"`
	assertContains(t, page, "NPC Maker Check", wantField, `<a action="bypass admin_maker 2">02</a>`)
	if strings.Contains(page, "admin_away") || strings.Contains(page, "grouped_000") {
		t.Fatalf("first page = %q, want only admin_field", page)
	}
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBypass("admin_maker 2"))),
		`<font color=LEVEL>grouped_000</font>`, "min=-2000, max=500",
		`<a action="bypass -h admin_maker admin_grp">admin_grp</a>`)

	// One maker's panel; the name matches ignoring case.
	info := pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker ADMIN_M1")))
	assertContains(t, info, "NPC Maker Info",
		"<td>admin_m1</td>", "<td width=200>admin_field</td>", "<td>default_maker</td>", "<td>true</td>",
		`(<font color="00FF00">2</font> + <font color="FF0000">0</font>) / 2`,
		`<tr><td>21101</td><td>Maker Admin Wolf</td><td>2 / 2</td><td> 1m 30s /  30s</td></tr></table>`)
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker admin_ev"))),
		"<td>admin_event</td>", "<td>false</td>", `(<font color="00FF00">0</font> + <font color="FF0000">0</font>) / 1`,
		`<td>0 / 1</td><td>1h /  0s</td>`)
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker admin_grp"))),
		"<td width=200>grouped_000</td>", "<td>default_maker</td>")

	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker scripts admin_m1"))),
		"NPC Maker Scripts", "<td width=200>admin_m1</td>", "This NPC maker isn't affected by scripts.")

	// Delete takes the maker's NPCs out of the world before its panel.
	frames := exchangeZones(t, c, encodeBuildCmd("maker delete admin_m1"))
	if n := countOpcode(frames, serverpackets.OpcodeDeleteObject); n != 2 {
		t.Fatalf("delete frames = %x, want two DeleteObject", testsupport.FrameOpcodes(frames))
	}
	assertContains(t, pageOf(t, frames), `(<font color="00FF00">0</font> + <font color="FF0000">0</font>) / 2`)
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker respawn admin_m1"))), "<td>admin_m1</td>")
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker switch admin_m1"))), "<td>admin_m1</td>")

	// Spawn sends the maker its script event 1001.
	assertContains(t, pageOf(t, exchangeZones(t, c, encodeBuildCmd("maker spawn admin_m1"))), "<td>admin_m1</td>")
	if got := events.list(); !slices.Contains(got, "admin_m1:1001") {
		t.Fatalf("maker events = %q, want admin_m1:1001", got)
	}

	// The NPCs the spawn event places show up on their own time.
	assertTexts(t, onlyMessages(exchangeZones(t, c, encodeBuildCmd("maker nobody"))), `Maker "nobody" doesn't exist.`)
	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("maker delete nobody")), `Maker "nobody" doesn't exist.`)
	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("maker bogus admin_m1")), "Usage: //maker [delete|respawn|scripts|spawn|switch <name>]")
	// A page number that shows no page answers nothing.
	if frames := exchangeZones(t, c, encodeBuildCmd("maker 0")); len(frames) != 0 {
		t.Fatalf("//maker 0 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
}

func TestAdminSpawnAndDespawnEvent(t *testing.T) {
	c := bootMakerAdmin(t)

	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("spawn_event")), "Usage: //spawn_event <event_name>")
	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("despawn_event")), "Usage: //despawn_event <event_name>")

	// A maker of a listed event spawned at boot; despawning and spawning
	// it again count its one NPC, and a full maker spawns no more.
	assertTexts(t, onlyMessages(exchangeZones(t, c, encodeBuildCmd("despawn_event 18age"))), `Despawned 1 "18age" NPCs.`)
	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("despawn_event 18age")), `Despawned 0 "18age" NPCs.`)
	assertTexts(t, onlyMessages(exchangeZones(t, c, encodeBuildCmd("spawn_event 18age"))), `Spawned 1 "18age" NPCs.`)
	assertTexts(t, onlyMessages(exchangeZones(t, c, encodeBuildCmd("spawn_event 18age"))), `Spawned 1 "18age" NPCs.`)
	// The event name matches exactly.
	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("spawn_event 18AGE")), `Spawned 0 "18AGE" NPCs.`)
	// An event that is not listed keeps its makers' NPCs out.
	assertTexts(t, onlyMessages(exchangeZones(t, c, encodeBuildCmd("spawn_event admin_event"))), `Spawned 0 "admin_event" NPCs.`)
}

func TestAdminTerritory(t *testing.T) {
	c := bootMakerAdmin(t)

	assertTexts(t, exchangeZones(t, c, encodeBuildCmd("terr goto nowhere")), `Territory "nowhere" doesn't exist.`)
	if frames := exchangeZones(t, c, encodeBuildCmd("terr bogus admin_field")); len(frames) != 0 {
		t.Fatalf("unknown subcommand frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	if frames := exchangeZones(t, c, encodeBuildCmd("terr bogus")); len(frames) != 0 {
		t.Fatalf("unknown word frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	assertPage(t, exchangeZones(t, c, encodeBuildCmd("terr")), "NPC Maker Check")

	// Visualize outlines each triangle in yellow at the character's height,
	// every side labelled with the territory name.
	frames := exchangeZones(t, c, encodeBuildCmd("terr visual admin_field"))
	if len(frames) != 2 {
		t.Fatalf("visual frames = %x, want the drawing then the page", testsupport.FrameOpcodes(frames))
	}
	p := decodePrimitive(t, frames[0])
	z := int32(gameservertest.SpawnZ)
	sides := undirected(t, p, "TERR")
	want := []string{
		segment(-1000, -1000, 1000, -1000, z), segment(-1000, -1000, 0, 1000, z), segment(1000, -1000, 0, 1000, z),
	}
	slices.Sort(want)
	if !slices.Equal(sides, want) {
		t.Fatalf("visual sides = %q, want %q", sides, want)
	}
	for _, s := range p.strokes {
		if s.label != "admin_field" || s.rgb != showYellow || !s.colored {
			t.Fatalf("visual stroke = %+v, want a yellow admin_field label", s)
		}
	}
	assertPage(t, frames[1:], "NPC Maker Check")

	// The merged territory answers to its grouped name: a square and the
	// far triangle, three triangles in all.
	frames = exchangeZones(t, c, encodeBuildCmd("terr visual grouped_000"))
	if got := len(decodePrimitive(t, frames[0]).strokes); got != 9 {
		t.Fatalf("grouped visual strokes = %d, want 9", got)
	}

	// Test samples 10000 points inside the territory.
	frames = exchangeZones(t, c, encodeBuildCmd("terr test admin_west"))
	points := 0
	for _, f := range frames[:len(frames)-1] {
		p := decodePrimitive(t, f)
		if !strings.HasPrefix(p.name, "TERR") {
			t.Fatalf("test drawing = %q, want TERR", p.name)
		}
		for _, s := range p.strokes {
			x, y := s.at[0], s.at[1]
			if s.line || s.label != "" || s.rgb != showGreen || s.colored || x < -200 || x > 200 || y < -200 || y > 200 {
				t.Fatalf("test stroke = %+v, want an unlabelled green point in admin_west", s)
			}
			points++
		}
	}
	if points != 10000 {
		t.Fatalf("test points = %d, want 10000", points)
	}
	assertPage(t, frames[len(frames)-1:], "NPC Maker Check")

	// Clearing blanks every part the sample filled, then the page.
	frames = exchangeZones(t, c, encodeBuildCmd("terr visual_clear"))
	if len(frames) < 2 {
		t.Fatalf("visual_clear frames = %x, want the blank drawing then the page", testsupport.FrameOpcodes(frames))
	}
	for i, f := range frames[:len(frames)-1] {
		name := "TERR"
		if i > 0 {
			name += strconv.Itoa(i)
		}
		assertBlank(t, decodePrimitive(t, f), name)
	}
	assertPage(t, frames[len(frames)-1:], "NPC Maker Check")

	// Go to teleports into the territory, then the page.
	frames = exchangeZones(t, c, encodeBuildCmd("terr goto admin_west"))
	var to [3]int32
	found := false
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation {
			_, to = teleportTo(t, f)
			found = true
		}
	}
	if !found || to[0] < -200 || to[0] > 200 || to[1] < -200 || to[1] > 200 {
		t.Fatalf("goto frames = %x (to %v), want a teleport into admin_west", testsupport.FrameOpcodes(frames), to)
	}
}

func segment(x1, y1, x2, y2, z int32) string {
	a, b := [3]int32{x1, y1, z}, [3]int32{x2, y2, z}
	if slices.Compare(a[:], b[:]) > 0 {
		a, b = b, a
	}
	return fmt.Sprint(a, b)
}

// undirected returns p's line strokes as sorted undirected segments,
// requiring p to be drawing name.
func undirected(t *testing.T, p primitive, name string) []string {
	t.Helper()
	if p.name != name {
		t.Fatalf("drawing = %q, want %q", p.name, name)
	}
	var out []string
	for _, s := range p.strokes {
		if !s.line {
			t.Fatalf("stroke = %+v, want a line", s)
		}
		out = append(out, segment(s.at[0], s.at[1], s.at[3], s.at[4], s.at[2]))
		if s.at[2] != s.at[5] {
			t.Fatalf("line %+v leaves its height", s)
		}
	}
	slices.Sort(out)
	return out
}

func onlyMessages(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, f)
		}
	}
	return out
}
