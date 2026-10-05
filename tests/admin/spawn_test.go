package admin

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// Template ids of the spawn fixtures.
const (
	wolfID     = 20120
	grocerID   = 30001
	artifactID = 35063
	treeID     = 13006
)

// spawnTemplates are a monster, a merchant, a castle artifact (no live
// model yet) and a Christmas tree.
func spawnTemplates() *npc.Table {
	tmpl := func(id int, typ, name string) *npc.Template {
		return &npc.Template{ID: id, TemplateID: id, Type: typ, Name: name, Level: 1, HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true}
	}
	wolf := tmpl(wolfID, "Monster", "Wolf")
	// A wolf leads the privates its spawn declares.
	wolf.AIParams = npc.AIParams{"Party_Type": "2"}
	return npc.NewTable([]*npc.Template{
		wolf,
		tmpl(grocerID, "Merchant", "Grocer Lector"),
		tmpl(artifactID, "HolyThing", "Artifact"),
		tmpl(treeID, "ChristmasTree", "Christmas Tree"),
	})
}

// bootSpawnAdmin boots an admin in the world with a live NPC population of
// spawnTemplates spawning makers (none when nil).
func bootSpawnAdmin(t *testing.T, makers *spawn.Table) (*gameservertest.Server, int32) {
	t.Helper()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithNPCs(spawnTemplates()), gameservertest.WithNpcSpawns(makers))
	enterWorld(t, srv.Client)
	return srv, gmID
}

// wolfDen is a maker of one wolf with one wolf private.
func wolfDen(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="wolf_den" territory="den" maximumNpcs="1">
		<npc id="20120" total="1" pos="200;300;30;0" respawn="60sec"><privates><private id="20120" weight="1" respawn="60sec"/></privates></npc>
	</npcmaker>
</list>`
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// npcsOf returns the world objects that are NPCs of template id, by object
// id.
func npcsOf(srv *gameservertest.Server, id int) []world.Tracked {
	var out []world.Tracked
	for _, obj := range srv.State.Objects() {
		var inst *npc.Instance
		switch o := obj.(type) {
		case *npc.Hostile:
			inst = o.Instance
		case *npc.Folk:
			inst = o.Instance
		case *npc.Decoration:
			inst = o.Instance
		}
		if inst != nil && inst.Template.ID == id {
			out = append(out, obj)
		}
	}
	slices.SortFunc(out, func(a, b world.Tracked) int { return cmp.Compare(a.ObjectID(), b.ObjectID()) })
	return out
}

type placed interface {
	Position() (x, y, z int)
	Heading() int
}

// textsIn returns the plain-text system messages among frames.
func textsIn(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id, text := systemText(t, frame); id == serverpackets.SystemMessageS1 {
			out = append(out, text)
		}
	}
	return out
}

// missingSpawnsPage is the admin page sent for spawns.htm, which the
// datapack does not ship.
const missingSpawnsPage = "<html><body>My html is missing:<br>data/html/admin/spawns.htm</body></html>"

// TestAdminSpawn pins AdminSpawn.java's //spawn: the NPC of an id, or of a
// name with underscores for spaces, matched case-insensitively, is placed
// where the GM's selection (the GM without one) stands, facing its way, and
// the GM is told "You spawned <name>. - Cmd: admin_spawn". Missing or
// unreadable arguments open spawns.htm; an unknown template, or one that
// is no NPC Go can place, answers APPLICANT_INFORMATION_INCORRECT (12).
func TestAdminSpawn(t *testing.T) {
	t.Parallel()
	srv, gmID := bootSpawnAdmin(t, nil)
	gm := srv.Client

	for _, cmd := range []string{"spawn", "spawn 20120 soon", "spawn 99999999999"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 || htmlBody(t, frames[0]) != missingSpawnsPage {
			t.Fatalf("//%s frames = %x, want the spawns.htm page", cmd, testsupport.FrameOpcodes(frames))
		}
	}
	for _, cmd := range []string{"spawn 4242", "spawn No_Such_Npc", "spawn 35063"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want APPLICANT_INFORMATION_INCORRECT", cmd, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], 12)
	}
	if got := npcsOf(srv, artifactID); len(got) != 0 {
		t.Fatalf("refused //spawn placed %d artifacts", len(got))
	}

	frames := exchange(t, gm, encodeBuildCmd("spawn 20120 30"))
	if got := textsIn(t, frames); !slices.Equal(got, []string{"You spawned Wolf. - Cmd: admin_spawn"}) {
		t.Fatalf("//spawn 20120 messages = %q, want the spawn notice", got)
	}
	wolves := npcsOf(srv, wolfID)
	if len(wolves) != 1 {
		t.Fatalf("wolves in the world = %d, want 1", len(wolves))
	}
	gmObj, _ := srv.State.Object(gmID)
	wolf := wolves[0].(placed)
	if x, y, z := wolf.Position(); x != spawnX || y != spawnY || z != spawnZ || wolf.Heading() != gmObj.(placed).Heading() {
		t.Fatalf("wolf at %d,%d,%d heading %d; want the GM's spot and facing", x, y, z, wolf.Heading())
	}

	// With the wolf selected, the next NPC stands where the wolf does.
	exchange(t, gm, encodeAction(wolves[0].ObjectID()))
	frames = exchange(t, gm, encodeBuildCmd("spawn grocer_LECTOR"))
	if got := textsIn(t, frames); !slices.Equal(got, []string{"You spawned Grocer Lector. - Cmd: admin_spawn"}) {
		t.Fatalf("//spawn grocer_LECTOR messages = %q, want the spawn notice", got)
	}
	grocers := npcsOf(srv, grocerID)
	if len(grocers) != 1 {
		t.Fatalf("grocers in the world = %d, want 1", len(grocers))
	}
	if _, ok := grocers[0].(*npc.Folk); !ok {
		t.Fatalf("grocer is %T, want a civilian NPC", grocers[0])
	}
	wx, wy, wz := wolf.Position()
	if x, y, z := grocers[0].(placed).Position(); x != wx || y != wy || z != wz {
		t.Fatalf("grocer at %d,%d,%d; want the selected wolf's spot %d,%d,%d", x, y, z, wx, wy, wz)
	}

	frames = exchange(t, gm, encodeBuildCmd("spawn 13006"))
	if got := textsIn(t, frames); !slices.Equal(got, []string{"You spawned Christmas Tree. - Cmd: admin_spawn"}) {
		t.Fatalf("//spawn 13006 messages = %q, want the spawn notice", got)
	}
	if trees := npcsOf(srv, treeID); len(trees) != 1 {
		t.Fatalf("trees in the world = %d, want 1", len(trees))
	} else if _, ok := trees[0].(*npc.Decoration); !ok {
		t.Fatalf("tree is %T, want a decoration", trees[0])
	}
}

// TestAdminDelete pins AdminSpawn.java's //delete: only an NPC of a
// standalone spawn — a //spawn, or an item's decoration — is removed, for
// good, with "You deleted <name>."; no selection, or an NPC no such spawn
// placed, is INVALID_TARGET (109) and stays.
func TestAdminDelete(t *testing.T) {
	t.Parallel()
	srv, _ := bootSpawnAdmin(t, nil)
	gm := srv.Client

	assertInvalidTarget := func(cmd string) {
		t.Helper()
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want INVALID_TARGET", cmd, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}
	assertInvalidTarget("delete")

	tmpl, _ := spawnTemplates().Get(wolfID)
	loose := srv.SpawnMovingHostileNPCTemplate(t, tmpl, location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ}, location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(loose.ObjectID()))
	assertInvalidTarget("delete")
	srv.Settle(t)
	if _, ok := srv.State.Object(loose.ObjectID()); !ok {
		t.Fatal("//delete removed an NPC no spawn placed")
	}

	for _, cmd := range []string{"spawn 20120", "spawn 30001", "spawn 13006"} {
		exchange(t, gm, encodeBuildCmd(cmd))
	}
	for _, tc := range []struct {
		id   int
		name string
	}{{wolfID, "Wolf"}, {grocerID, "Grocer Lector"}, {treeID, "Christmas Tree"}} {
		var target world.Tracked
		for _, obj := range npcsOf(srv, tc.id) {
			if obj.ObjectID() != loose.ObjectID() {
				target = obj
			}
		}
		if target == nil {
			t.Fatalf("no spawned %s", tc.name)
		}
		exchange(t, gm, encodeAction(target.ObjectID()))
		frames := exchange(t, gm, encodeBuildCmd("delete"))
		if got := textsIn(t, frames); !slices.Equal(got, []string{"You deleted " + tc.name + "."}) {
			t.Fatalf("//delete %s messages = %q (frames %x), want the delete notice", tc.name, got, testsupport.FrameOpcodes(frames))
		}
		srv.Settle(t)
		if _, ok := srv.State.Object(target.ObjectID()); ok {
			t.Fatalf("deleted %s still in the world", tc.name)
		}
	}
	if got := srv.NpcSpawns.LiveCount(); got != 0 {
		t.Fatalf("live spawned NPCs = %d, want none left to respawn", got)
	}
}

// listSpawnsRow is one row of //list_spawns as AdminSpawn.java writes it.
func listSpawnsRow(row int, x, y, z int, label string) string {
	open := "<table width=280 height=41><tr>"
	if row%2 == 0 {
		open = "<table width=280 height=41 bgcolor=000000><tr>"
	}
	return open + fmt.Sprintf(`<td><a action="bypass -h admin_teleport %d %d %d">%d`, x, y, z, row) + label +
		`</td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// onePageBar is Pagination.generatePages for page 1 of 1.
func onePageBar(action string) string {
	empty := strings.Repeat(`<td FIXWIDTH=26 align=center></td>`, 4)
	return `<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center><img height=2><button action="` + action + `" back=L2UI_CH3.prev1_down fore=L2UI_CH3.prev1 width=16 height=16></td>` +
		empty + `<td FIXWIDTH=26 align=center><font color=LEVEL>01</font></td>` + empty +
		`<td FIXWIDTH=22 align=center><img height=2><button action="` + action + `" back=L2UI_CH3.next1_down fore=L2UI_CH3.next1 width=16 height=16></td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// TestAdminListSpawns pins AdminSpawn.java's //list_spawns byte for byte:
// every NPC of the id or name (the selected NPC's without one), each row a
// teleport link numbered from eight times the page asked for less one,
// naming a standalone spawn and its spawn point, or the NPC's own position
// when no spawn placed it; empty rows pad the page, then the page bar. An
// unknown name, id 0 or no NPC selected is INVALID_TARGET; an unreadable
// page, or page 0 with NPCs listed, sends nothing.
func TestAdminListSpawns(t *testing.T) {
	t.Parallel()
	srv, _ := bootSpawnAdmin(t, wolfDen(t))
	gm := srv.Client

	for _, cmd := range []string{"list_spawns", "list_spawns 0", "list_spawns nobody"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want INVALID_TARGET", cmd, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}

	exchange(t, gm, encodeBuildCmd("spawn 20120"))
	tmpl, _ := spawnTemplates().Get(wolfID)
	at := location.Location{X: spawnX + 40, Y: spawnY - 10, Z: spawnZ}
	srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	drain(t, gm)
	// By object id: the maker's wolf, its private, the //spawn wolf, then
	// the wolf no spawn placed.
	wolves := npcsOf(srv, wolfID)
	if len(wolves) != 4 {
		t.Fatalf("wolves = %d, want 4", len(wolves))
	}
	den, private, spawned := wolves[0].ObjectID(), wolves[1].ObjectID(), wolves[2].(placed)
	sx, sy, sz := spawned.Position()
	labels := []string{
		" - MultiSpawn [id=20120]</a><br1>NpcMaker: wolf_den",
		fmt.Sprintf(" - Spawn [id=20120]</a><br1>Master: Wolf [objId=%d]", den),
		fmt.Sprintf(" - Spawn [id=20120]</a><br1>Location: %d, %d, %d, %d", sx, sy, sz, spawned.Heading()),
		fmt.Sprintf(" - (%d, %d, %d, %d)</a>", at.X, at.Y, at.Z, 0),
	}
	if rec, ok := srv.NpcSpawns.SpawnOf(private); !ok || rec.MasterID != den {
		t.Fatalf("SpawnOf(private) = %+v, %v; want master %d", rec, ok, den)
	}
	rows := func(first int) string {
		var b strings.Builder
		for i, obj := range wolves {
			x, y, z := obj.(placed).Position()
			b.WriteString(listSpawnsRow(first+i, x, y, z, labels[i]))
		}
		return b.String()
	}
	page := func(first int) string {
		return "<html><body>" + rows(first) + strings.Repeat("<img height=42>", 4) + onePageBar("bypass admin_list_spawns 20120 1") + "</body></html>"
	}

	for _, tc := range []struct {
		cmd   string
		first int
	}{{"list_spawns 20120", 0}, {"list_spawns WOLF 1", 0}, {"list_spawns 20120 2", 8}} {
		frames := exchange(t, gm, encodeBuildCmd(tc.cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want one page", tc.cmd, testsupport.FrameOpcodes(frames))
		}
		if got, want := htmlBody(t, frames[0]), page(tc.first); got != want {
			t.Fatalf("//%s page =\n%s\nwant\n%s", tc.cmd, got, want)
		}
	}

	exchange(t, gm, encodeAction(wolves[1].ObjectID()))
	frames := exchange(t, gm, encodeBuildCmd("list_spawns"))
	if len(frames) != 1 || htmlBody(t, frames[0]) != page(0) {
		t.Fatalf("//list_spawns on the selected wolf frames = %x, want the wolf page", testsupport.FrameOpcodes(frames))
	}

	for _, cmd := range []string{"list_spawns 20120 x", "list_spawns 20120 0", "list_spawns 99999999999"} {
		if frames := exchange(t, gm, encodeBuildCmd(cmd)); len(frames) != 0 {
			t.Fatalf("//%s frames = %x, want nothing", cmd, testsupport.FrameOpcodes(frames))
		}
	}
}

// TestAdminListSpawnsDecoration pins the //list_spawns row of a //spawn
// decoration: like any standalone Spawn it reads "Spawn [id=N]" over
// "Location: x, y, z, heading" of its spawn point (Spawn.toString,
// Spawn.getDescription), never the bare position of an NPC no spawn placed.
func TestAdminListSpawnsDecoration(t *testing.T) {
	t.Parallel()
	srv, _ := bootSpawnAdmin(t, nil)
	gm := srv.Client

	exchange(t, gm, encodeBuildCmd("spawn 13006"))
	trees := npcsOf(srv, treeID)
	if len(trees) != 1 {
		t.Fatalf("trees in the world = %d, want 1", len(trees))
	}
	if _, ok := trees[0].(*npc.Decoration); !ok {
		t.Fatalf("tree is %T, want a decoration", trees[0])
	}
	tree := trees[0].(placed)
	x, y, z := tree.Position()
	want := "<html><body>" +
		listSpawnsRow(0, x, y, z, fmt.Sprintf(" - Spawn [id=13006]</a><br1>Location: %d, %d, %d, %d", x, y, z, tree.Heading())) +
		strings.Repeat("<img height=42>", 7) + onePageBar("bypass admin_list_spawns 13006 1") + "</body></html>"

	frames := exchange(t, gm, encodeBuildCmd("list_spawns 13006"))
	if len(frames) != 1 {
		t.Fatalf("//list_spawns 13006 frames = %x, want one page", testsupport.FrameOpcodes(frames))
	}
	if got := htmlBody(t, frames[0]); got != want {
		t.Fatalf("//list_spawns 13006 page =\n%s\nwant\n%s", got, want)
	}
}
