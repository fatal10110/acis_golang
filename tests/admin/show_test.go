package admin

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const showUsage = "Usage : //show <clear|door|html|move|path|restart|walker>"

var (
	showGreen  = [3]int32{0, 255, 0}
	showRed    = [3]int32{255, 0, 0}
	showYellow = [3]int32{255, 255, 0}
)

func bootShowAdmin(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient) {
	t.Helper()
	opts = append([]gameservertest.Option{gameservertest.WithHTMLPages(shippedAdminPages(t, "walker.htm"))}, opts...)
	srv, _ := bootAdmin(t, adminLevel, opts...)
	enterWorld(t, srv.Client)
	return srv, srv.Client
}

// showLine is a labelled, name-coloured line stroke.
func showLine(label string, rgb [3]int32, from, to [3]int32) stroke {
	return stroke{line: true, label: label, rgb: rgb, colored: true, at: [6]int32{from[0], from[1], from[2], to[0], to[1], to[2]}}
}

// assertStrokes requires p to be the frame name at the spawn drawing
// exactly want, in any order: the client draws a frame's strokes as a set.
func assertStrokes(t *testing.T, p primitive, name string, want ...stroke) {
	t.Helper()
	if p.name != name || p.anchor != [3]int32{spawnX, spawnY, -16000} {
		t.Fatalf("drawing %q at %v, want %s at the spawn 16000 below", p.name, p.anchor, name)
	}
	key := func(s stroke) string { return fmt.Sprintf("%+v", s) }
	got, exp := make([]string, len(p.strokes)), make([]string, len(want))
	for i, s := range p.strokes {
		got[i] = key(s)
	}
	for i, s := range want {
		exp[i] = key(s)
	}
	slices.Sort(got)
	slices.Sort(exp)
	if !slices.Equal(got, exp) {
		t.Fatalf("%s strokes = %+v, want %+v", name, p.strokes, want)
	}
}

// TestAdminShowUsageAndSilence pins the answers that draw nothing: no or an
// unknown subcommand, or a page past int range, answers the usage; a
// restart without area or point (matched case-sensitively) answers
// nothing.
func TestAdminShowUsageAndSilence(t *testing.T) {
	t.Parallel()
	_, gm := bootShowAdmin(t)
	for _, cmd := range []string{"show", "show foo", "show walker 99999999999"} {
		assertTexts(t, exchangeZones(t, gm, encodeBuildCmd(cmd)), showUsage)
	}
	for _, cmd := range []string{"show restart", "show restart AREA", "show RESTART bogus"} {
		if frames := exchangeZones(t, gm, encodeBuildCmd(cmd)); len(frames) != 0 {
			t.Fatalf("//%s = %x, want nothing", cmd, opcodes(frames))
		}
	}
}

// TestAdminShowRefusedBelowAdmin pins the access check: a player below the
// command's level is told it lacks the rights and nothing is drawn.
func TestAdminShowRefusedBelowAdmin(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, 1)
	enterWorld(t, srv.Client)
	assertTexts(t, exchangeZones(t, srv.Client, encodeBuildCmd("show boat")), "You don't have the access right to use this command.")
}

// TestAdminShowBoat pins //show boat: each route of each itinerary a yellow
// segment between consecutive nodes, numbered from 0 per route, the last
// node closing none.
func TestAdminShowBoat(t *testing.T) {
	t.Parallel()
	at := func(x, y, z int) route.BoatLocation {
		return route.BoatLocation{Location: location.Location{X: x, Y: y, Z: z}, Speed: 200, Rotation: 800}
	}
	round := route.BoatItinerary{Routes: []route.BoatRoute{
		{Dock: route.DockRune, Nodes: []route.BoatLocation{at(1, 2, 3), at(4, 5, 6)}},
		{Dock: route.DockPrimeval, Nodes: []route.BoatLocation{at(7, 8, 9), at(10, 11, 12), at(13, 14, 15)}},
	}}
	oneWay := route.BoatItinerary{Routes: []route.BoatRoute{
		{Dock: route.DockGludin, Nodes: []route.BoatLocation{at(20, 21, 22), at(23, 24, 25)}},
	}}
	_, gm := bootShowAdmin(t, gameservertest.WithBoats(round, oneWay))
	frames := exchangeZones(t, gm, encodeBuildCmd("show BOAT"))
	if len(frames) != 1 {
		t.Fatalf("//show boat = %x, want one drawing", opcodes(frames))
	}
	assertStrokes(t, decodePrimitive(t, frames[0]), "BOAT",
		showLine("Segment #0", showYellow, [3]int32{1, 2, 3}, [3]int32{4, 5, 6}),
		showLine("Segment #0", showYellow, [3]int32{7, 8, 9}, [3]int32{10, 11, 12}),
		showLine("Segment #1", showYellow, [3]int32{10, 11, 12}, [3]int32{13, 14, 15}),
		showLine("Segment #0", showYellow, [3]int32{20, 21, 22}, [3]int32{23, 24, 25}),
	)
}

// TestAdminShowDoorAndDebugMove pins //show door: every door the game
// master knows outlined three times, 32 below its height green labelled
// MinZ, at its height yellow, 32 above red labelled MaxZ; a door out of
// sight is not drawn. With the door selected, //show move toggles its move
// debugging, telling the game master; //show clear on it clears nothing;
// turning the toggle off blanks the door's MOVE drawing for every game
// master.
func TestAdminShowDoorAndDebugMove(t *testing.T) {
	t.Parallel()
	gate := func(id, x int, name string) *door.Template {
		return &door.Template{
			ID: id, Name: name, Kind: door.KindDoor, Level: 1,
			Position:    location.Location{X: x, Y: 0, Z: -100},
			Coordinates: []location.Point{{X: x - 8, Y: -8}, {X: x + 8, Y: -8}, {X: x + 8, Y: 8}, {X: x - 8, Y: 8}},
			HP:          100, PDef: 10, MDef: 10, Height: 32,
		}
	}
	srv, gm := bootShowAdmin(t, gameservertest.WithDoors(gate(24190001, 40, "show_gate"), gate(24190002, 90000, "far_gate")))
	frames := exchangeZones(t, gm, encodeBuildCmd("show door"))
	if len(frames) != 1 {
		t.Fatalf("//show door = %x, want one drawing", opcodes(frames))
	}
	var want []stroke
	corners := [][2]int32{{32, -8}, {48, -8}, {48, 8}, {32, 8}}
	for i, c := range corners {
		n := corners[(i+1)%len(corners)]
		want = append(want,
			showLine("show_gate MinZ", showGreen, [3]int32{c[0], c[1], -132}, [3]int32{n[0], n[1], -132}),
			showLine("show_gate", showYellow, [3]int32{c[0], c[1], -100}, [3]int32{n[0], n[1], -100}),
			showLine("show_gate MaxZ", showRed, [3]int32{c[0], c[1], -68}, [3]int32{n[0], n[1], -68}),
		)
	}
	assertStrokes(t, decodePrimitive(t, frames[0]), "DOOR", want...)

	d, ok := srv.WorldObjects.Door(24190001)
	if !ok {
		t.Fatal("show_gate not spawned")
	}
	gm.Send(encodeAction(d.ObjectID()))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show move")), "Debug move enabled on show_gate")
	if frames := exchangeZones(t, gm, encodeBuildCmd("show clear")); len(frames) != 0 {
		t.Fatalf("//show clear on a door = %x, want nothing", opcodes(frames))
	}
	frames = exchangeZones(t, gm, encodeBuildCmd("show move"))
	if len(frames) != 2 {
		t.Fatalf("//show move off = %x, want the message then the blanked drawing", opcodes(frames))
	}
	assertTexts(t, frames[:1], "Debug move disabled on show_gate")
	assertBlank(t, decodePrimitive(t, frames[1]), "MOVE"+strconv.Itoa(int(d.ObjectID())))
}

// TestAdminShowRestartAndClear pins //show restart area (every restart area
// outlined, unlabelled) and //show restart point (spots green, chaotic spots
// red), then //show clear: every drawing the game master holds blanked, by
// name, and forgotten, so a second clear sends nothing.
func TestAdminShowRestartAndClear(t *testing.T) {
	t.Parallel()
	area, err := restart.NewArea([]location.Point{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 0, Y: 100}}, -100, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	table := zoneTestRestarts()
	table.Areas = []restart.Area{area}
	table.Points[0].Points = []location.Location{{X: 1, Y: 2, Z: 3}}
	table.Points[0].ChaoPoints = []location.Location{{X: 4, Y: 5, Z: 6}}
	_, gm := bootShowAdmin(t, gameservertest.WithRestartPoints(table))

	frames := exchangeZones(t, gm, encodeBuildCmd("show restart area"))
	if len(frames) != 1 {
		t.Fatalf("//show restart area = %x, want one drawing", opcodes(frames))
	}
	var want []stroke
	nodes := [][2]int32{{0, 0}, {100, 0}, {0, 100}}
	for i, c := range nodes {
		n := nodes[(i+1)%len(nodes)]
		want = append(want,
			showLine("", showGreen, [3]int32{c[0], c[1], -132}, [3]int32{n[0], n[1], -132}),
			showLine("", showRed, [3]int32{c[0], c[1], 68}, [3]int32{n[0], n[1], 68}),
			showLine("", showYellow, [3]int32{c[0], c[1], -132}, [3]int32{c[0], c[1], 68}),
		)
	}
	assertStrokes(t, decodePrimitive(t, frames[0]), "RESTART_AREA", want...)

	frames = exchangeZones(t, gm, encodeBuildCmd("show restart point"))
	if len(frames) != 1 {
		t.Fatalf("//show restart point = %x, want one drawing", opcodes(frames))
	}
	assertStrokes(t, decodePrimitive(t, frames[0]), "RESTART_POINT",
		stroke{label: "point", rgb: showGreen, colored: true, at: [6]int32{1, 2, 3}},
		stroke{label: "chaoPoint", rgb: showRed, colored: true, at: [6]int32{4, 5, 6}},
	)

	frames = exchangeZones(t, gm, encodeBuildCmd("show clear"))
	if len(frames) != 2 {
		t.Fatalf("//show clear = %x, want both drawings blanked", opcodes(frames))
	}
	assertBlank(t, decodePrimitive(t, frames[0]), "RESTART_AREA")
	assertBlank(t, decodePrimitive(t, frames[1]), "RESTART_POINT")
	if frames := exchangeZones(t, gm, encodeBuildCmd("show clear")); len(frames) != 0 {
		t.Fatalf("second //show clear = %x, want nothing", opcodes(frames))
	}
}

// TestAdminShowPathOnPlayer pins //show path on a selected player: the game
// master and the player are told; turned off, the player's PATH drawing is
// blanked for every game master, the player included only as itself. On
// the game master itself, off blanks its drawing twice: as a game master
// and as the creature.
func TestAdminShowPathOnPlayer(t *testing.T) {
	t.Parallel()
	srv, gm := bootShowAdmin(t)
	gmID := srv.SoleObjectID(t)
	other, otherID := addPlayer(t, srv, "showother", "Other", userLevel)
	watcher, _ := addPlayer(t, srv, "showwatch", "Watcher", adminLevel)
	drain(t, gm)
	gm.Send(encodeAction(otherID))
	drain(t, gm)
	drain(t, other)
	drain(t, watcher)

	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show path")), "Debug path enabled on Other")
	assertTexts(t, exchangeZones(t, other, nil), "Debug path was enabled.")

	// The commanding game master is a game master too.
	drawing := "PATH" + strconv.Itoa(int(otherID))
	frames := exchangeZones(t, gm, encodeBuildCmd("show path"))
	if len(frames) != 2 {
		t.Fatalf("//show path off = %x, want the message then the blanked drawing", opcodes(frames))
	}
	assertTexts(t, frames[:1], "Debug path disabled on Other")
	assertBlank(t, decodePrimitive(t, frames[1]), drawing)
	frames = exchangeZones(t, other, nil)
	if len(frames) != 2 {
		t.Fatalf("player frames = %x, want the message then its blanked drawing", opcodes(frames))
	}
	assertTexts(t, frames[:1], "Debug path was disabled.")
	assertBlank(t, decodePrimitive(t, frames[1]), drawing)
	frames = exchangeZones(t, watcher, nil)
	if len(frames) != 1 {
		t.Fatalf("watching game master frames = %x, want the blanked drawing", opcodes(frames))
	}
	assertBlank(t, decodePrimitive(t, frames[0]), drawing)

	gm.Send(encodeAction(gmID))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show move")), "Debug move enabled on Admin")
	frames = exchangeZones(t, gm, encodeBuildCmd("show move"))
	if len(frames) != 3 {
		t.Fatalf("//show move off on self = %x, want the message then the drawing blanked twice", opcodes(frames))
	}
	assertTexts(t, frames[:1], "Debug move disabled on Admin")
	self := "MOVE" + strconv.Itoa(int(gmID))
	assertBlank(t, decodePrimitive(t, frames[1]), self)
	assertBlank(t, decodePrimitive(t, frames[2]), self)
}

// TestAdminShowWalker pins //show walker: the panel lists every walker
// route by route then NPC name, unspawned or with the NPC walking it, the
// node it heads for and its direction; a route named for itself is drawn
// as a closed yellow loop numbered from 1 before the panel opens at the
// asked page; a name no route has is not linked, a route without that NPC
// name answers the usage, as does page 0.
func TestAdminShowWalker(t *testing.T) {
	t.Parallel()
	const porterID, alias = 31356, "porter_remy"
	tmpl := gameservertest.FolkTemplate("Folk", porterID)
	tmpl.Alias, tmpl.Name, tmpl.CanMove = alias, "Porter Remy", true
	a, b := location.Location{X: 110, Y: 20, Z: 50}, location.Location{X: 410, Y: 20, Z: 50}
	routes := route.WalkerRoutes{
		alias:  {alias: {{Location: a}, {Location: b}}},
		"zeta": {"ghost": {{Location: location.Location{X: 7, Y: 8, Z: 9}}}},
	}
	srv, gm := bootShowAdmin(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithWalkerRoutes(routes),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{porterID: script.KindFolk},
			[]script.Listing{{Path: "script.ai.group.Walkers"}}, script.Catalog{"script.ai.group.Walkers": group.Walkers}),
		gameservertest.WithAITask(),
	)
	const pages = `<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center>`
	unspawned := `<table width=280 bgcolor=000000><tr><td width=150><a action="bypass admin_teleport 110 20 50">Unspawned</a></td><td width=40>-</td><td width=50 align=right>-</td><td width=40 align=right><a action="bypass admin_show walker 1 porter_remy">Show</a></td></tr></table><img src="L2UI.SquareGray" width=280 height=1>` +
		`<table width=280><tr><td width=150><a action="bypass admin_teleport 7 8 9">Unspawned</a></td><td width=40>-</td><td width=50 align=right>-</td><td width=40 align=right><a action="bypass admin_show walker 1 ghost">Show</a></td></tr></table><img src="L2UI.SquareGray" width=280 height=1>` +
		strings.Repeat("<img height=20>", 13) + pages
	frames := exchangeZones(t, gm, encodeBuildCmd("show walker"))
	if len(frames) != 1 || !strings.Contains(htmlBody(t, frames[0]), unspawned) {
		t.Fatalf("//show walker = %x, want the panel listing\n%s", opcodes(frames), unspawned)
	}
	if !strings.Contains(htmlBody(t, frames[0]), `bypass -h admin_show clear`) {
		t.Fatal("walker panel is not the shipped page")
	}

	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show walker ghost")), "The npcId ghost isn't linked to any WalkerRoute.")
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show walker zeta")), showUsage)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show walker 0")), showUsage)

	frames = exchangeZones(t, gm, encodeBuildCmd("show walker 3 porter_remy"))
	if len(frames) != 2 || frames[1][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("//show walker 3 porter_remy = %x, want the drawing then the panel", opcodes(frames))
	}
	assertStrokes(t, decodePrimitive(t, frames[0]), "WALKER",
		showLine("Segment #1", showYellow, [3]int32{110, 20, 50}, [3]int32{410, 20, 50}),
		showLine("Segment #2", showYellow, [3]int32{410, 20, 50}, [3]int32{110, 20, 50}),
	)
	if page := htmlBody(t, frames[1]); !strings.Contains(page, `"bypass admin_show walker 3 porter_remy">Show`) {
		t.Fatalf("panel at page 3 does not link its rows back to page 3:\n%s", page)
	}

	if _, err := srv.NpcSpawns.AddSpawn(porterID, 160, 20, 30, 0, false, 0); err != nil {
		t.Fatalf("spawn porter: %v", err)
	}
	// The porter takes up its route on its AI's second tick.
	for range 2 {
		if err := srv.AI.Tick(); err != nil {
			t.Fatalf("AI tick: %v", err)
		}
	}
	drain(t, gm)
	walking := `<table width=280 bgcolor=000000><tr><td width=150><a action="bypass admin_teleport 110 20 50">Porter Remy</a></td><td width=40>0 / 2</td><td width=50 align=right>Regular</td><td width=40 align=right><a action="bypass admin_show walker 1 porter_remy">Show</a></td></tr></table>`
	frames = exchangeZones(t, gm, encodeBuildCmd("show walker 1"))
	if len(frames) != 1 || !strings.Contains(htmlBody(t, frames[0]), walking) {
		t.Fatalf("//show walker with the porter walking = %x, want the row\n%s\ngot\n%s", opcodes(frames), walking, htmlBody(t, frames[0]))
	}
}
