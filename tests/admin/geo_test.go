package admin

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	geoUsage  = "Usage: //geo bug|pos|see|move|fly"
	pathUsage = "Usage: //path find|info"
	// geoBeyondWallX is east of the wall wallGeo raises: geodata column 3
	// of the spawn's block (world x 48 to 63) closes its east edge, over
	// the block's eight rows (world y 0 to 127).
	geoBeyondWallX = 100
	// geoZ is the geodata's ground, where the game master stands: a
	// complex cell's height is a multiple of 8.
	geoZ = 32
)

var (
	showBlue    = [3]int32{0, 0, 255}
	showCyan    = [3]int32{0, 255, 255}
	showMagenta = [3]int32{255, 0, 255}
	showWhite   = [3]int32{255, 255, 255}
)

// wallGeo is flat ground at geoZ over the spawn's geodata region, the
// spawn's block open complex cells but for the wall on column 3's east
// edge.
func wallGeo(t *testing.T) *engine.Engine {
	t.Helper()
	eng := engine.New()
	b := block.NewRegionBuilder()
	for i := range block.RegionBlockCount {
		b.SetFlat(i, geoZ)
	}
	var cells [block.CellCount]block.Cell
	for x := range block.CellsX {
		for y := range block.CellsY {
			nswe := block.AllDirections
			if x == 3 {
				nswe &^= block.East
			}
			cells[x*block.CellsY+y] = block.Cell{Height: geoZ, NSWE: nswe}
		}
	}
	b.SetComplex(0, cells)
	rx, ry := engine.RegionOf(spawnX, spawnY)
	if err := eng.SetRegion(rx, ry, b.Build()); err != nil {
		t.Fatal(err)
	}
	if engine.GeoX(0)%block.CellsX != 0 || engine.GeoX(0)%(block.CellsX*block.RegionBlocksX) != 0 || engine.GeoY(0)%(block.CellsY*block.RegionBlocksY) != 0 {
		t.Fatal("world origin is not a region's first block")
	}
	return eng
}

// bootGeoAdmin boots the game master on wallGeo, standing on its ground,
// saving bug reports to a temporary directory it returns.
func bootGeoAdmin(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, string) {
	t.Helper()
	eng := wallGeo(t)
	dir := t.TempDir()
	bugs := engine.NewBugLog(dir)
	t.Cleanup(func() { _ = bugs.Close() })
	srv, objID := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(shippedAdminPages(t, "walker.htm")),
		gameservertest.WithGeo(move.NewGeo(eng, pathfind.New(eng, pathfind.DefaultOptions()))),
		gameservertest.WithGeoBugLog(bugs))
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET z = ? WHERE obj_Id = ?", geoZ, objID); err != nil {
		t.Fatal(err)
	}
	enterWorld(t, srv.Client)
	return srv, srv.Client, dir
}

// addPlayerAt is addPlayer entering the world at x, y, z.
func addPlayerAt(t *testing.T, srv *gameservertest.Server, account, name string, x, y, z int) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, account, name, 1, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", x, y, z, ch.ID); err != nil {
		t.Fatal(err)
	}
	c := srv.DialClient(t, account, 1)
	enterWorld(t, c)
	return c, ch.ID
}

// strokesLabelled returns p's strokes labelled label.
func strokesLabelled(p primitive, label string) []stroke {
	var out []stroke
	for _, s := range p.strokes {
		if s.label == label {
			out = append(out, s)
		}
	}
	return out
}

// TestAdminGeoUsageAndNoGeodata pins the answers without geodata: no
// subcommand answers the usage, an unknown one nothing; //geo pos names
// the region and a null block, then that there is no geodata, as does
// //geo bug; //path find without a selection is an invalid target, and
// with one but no path finder finds no route; //path info, like an unknown
// //path subcommand, answers nothing.
func TestAdminGeoUsageAndNoGeodata(t *testing.T) {
	t.Parallel()
	srv, gm := bootShowAdmin(t)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo")), geoUsage)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("path")), pathUsage)
	for _, cmd := range []string{"geo foo", "geo POS", "path info", "path foo"} {
		if frames := exchangeZones(t, gm, encodeBuildCmd(cmd)); len(frames) != 0 {
			t.Fatalf("//%s = %x, want nothing", cmd, opcodes(frames))
		}
	}
	rx, ry := engine.RegionOf(spawnX, spawnY)
	region := "Region: " + itoa(rx) + "_" + itoa(ry) + "; Block: BlockNull"
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo pos")), region, "There is no geodata at this position.")
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo bug here")), "There is no geodata at this position.")

	frames := exchangeZones(t, gm, encodeBuildCmd("path find"))
	if len(frames) != 1 {
		t.Fatalf("//path find without selection = %x, want one message", opcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	gm.Send(encodeAction(srv.SoleObjectID(t)))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("path find")), "No route found or pathfinding is disabled.")
}

// TestAdminGeoPosAndBug pins //geo pos over open geodata (the region and
// block, the open directions as a compass with the geodata coordinates,
// then the cell outlined green and gm marked red) and //geo bug: without a
// comment the usage, with one the report line appended to the geodata bug
// file, signed, any ';' written as ':'.
func TestAdminGeoPosAndBug(t *testing.T) {
	t.Parallel()
	_, gm, dir := bootGeoAdmin(t)
	frames := exchangeZones(t, gm, encodeBuildCmd("geo pos"))
	if len(frames) != 5 {
		t.Fatalf("//geo pos = %x, want four messages and a drawing", opcodes(frames))
	}
	rx, ry := engine.RegionOf(spawnX, spawnY)
	gx, gy := engine.GeoX(spawnX), engine.GeoY(spawnY)
	assertTexts(t, frames[:4],
		"Region: "+itoa(rx)+"_"+itoa(ry)+"; Block: BlockComplex",
		"    x x x          GeoX="+itoa(gx),
		"    x o x          GeoY="+itoa(gy),
		"    x x x          GeoZ="+itoa(geoZ),
	)
	cx, cy := int32(spawnX&^15), int32(spawnY&^15)
	assertStrokes(t, decodePrimitive(t, frames[4]), "POS",
		stroke{line: true, rgb: showGreen, at: [6]int32{cx, cy, geoZ, cx, cy + 15, geoZ}},
		stroke{line: true, rgb: showGreen, at: [6]int32{cx + 15, cy + 15, geoZ, cx, cy + 15, geoZ}},
		stroke{line: true, rgb: showGreen, at: [6]int32{cx + 15, cy + 15, geoZ, cx + 15, cy, geoZ}},
		stroke{line: true, rgb: showGreen, at: [6]int32{cx, cy, geoZ, cx + 15, cy, geoZ}},
		stroke{label: "POS", rgb: showRed, colored: true, at: [6]int32{spawnX, spawnY, geoZ}},
	)

	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo bug")), "Usage: //geo bug comments")
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo bug stuck; here")), "GeoData bug saved.")
	data, err := os.ReadFile(filepath.Join(dir, engine.BugLogFile))
	if err != nil {
		t.Fatal(err)
	}
	if want := engine.BugReport(spawnX, spawnY, geoZ, "Admin: stuck: here"); string(data) != want {
		t.Fatalf("bug file = %q, want %q", data, want)
	}
}

// TestAdminGeoMoveAndPath pins //geo move (whether gm walks straight to
// its selection, told but never drawn: a later //show clear blanks the
// walk) and //path find (each point of the route told, then the route
// drawn).
func TestAdminGeoMoveAndPath(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootGeoAdmin(t)
	frames := exchangeZones(t, gm, encodeBuildCmd("geo move"))
	if len(frames) != 1 {
		t.Fatalf("//geo move without selection = %x, want one message", opcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)

	gm.Send(encodeAction(srv.SoleObjectID(t)))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo move")), "Can move beeline.")

	_, otherID := addPlayerAt(t, srv, "geoother", "Other", geoBeyondWallX, spawnY, geoZ)
	drain(t, gm)
	gm.Send(encodeAction(otherID))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("geo move")), "Can not move beeline!")

	frames = exchangeZones(t, gm, encodeBuildCmd("path find"))
	if len(frames) < 2 {
		t.Fatalf("//path find = %x, want the route's points then its drawing", opcodes(frames))
	}
	// The route may cross the geodata-less region south of the spawn's,
	// whose ground is 0.
	point := regexp.MustCompile(`^x:-?\d+ y:-?\d+ z:-?\d+$`)
	for _, f := range frames[:len(frames)-1] {
		if _, text := systemText(t, f); !point.MatchString(text) {
			t.Fatalf("route point %q, want x:<x> y:<y> z:<z>", text)
		}
	}
	if _, last := systemText(t, frames[len(frames)-2]); last != "x:"+itoa(geoBeyondWallX&^15+8)+" y:"+itoa(spawnY&^15+8)+" z:"+itoa(geoZ) {
		t.Fatalf("last route point %q, want the selection's cell", last)
	}
	drawing := decodePrimitive(t, frames[len(frames)-1])
	if drawing.name != "PATH" {
		t.Fatalf("drawing %q, want PATH", drawing.name)
	}
	timing := regexp.MustCompile(`^\d+ms$`)
	timed := false
	for _, s := range drawing.strokes {
		timed = timed || (!s.line && s.rgb == showRed && timing.MatchString(s.label))
	}
	if !timed {
		t.Fatalf("PATH strokes %+v lack the red search time", drawing.strokes)
	}

	// //show clear clears the selected player's drawings: the unsent walk
	// is blanked with the route.
	gm.Send(encodeAction(srv.SoleObjectID(t)))
	drain(t, gm)
	frames = exchangeZones(t, gm, encodeBuildCmd("show clear"))
	if len(frames) != 2 {
		t.Fatalf("//show clear = %x, want the unsent walk and the route blanked", opcodes(frames))
	}
	assertBlank(t, decodePrimitive(t, frames[0]), "CAN_MOVE")
	assertBlank(t, decodePrimitive(t, frames[1]), "PATH")
}

// TestAdminGeoSeeAndFly pins //geo see (both bodies blue, the cells the
// sight walks both ways, the line of sight green with the obstacle
// allowance magenta above it) and //geo fly (the flight corridor 32 below
// both, green throughout when open).
func TestAdminGeoSeeAndFly(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootGeoAdmin(t)
	_, otherID := addPlayerAt(t, srv, "geoother", "Other", geoBeyondWallX, spawnY, geoZ)
	drain(t, gm)
	gm.Send(encodeAction(otherID))
	drain(t, gm)

	frames := exchangeZones(t, gm, encodeBuildCmd("geo see"))
	if len(frames) != 1 {
		t.Fatalf("//geo see = %x, want one drawing", opcodes(frames))
	}
	see := decodePrimitive(t, frames[0])
	if see.name != "CAN_SEE" {
		t.Fatalf("drawing %q, want CAN_SEE", see.name)
	}
	origin, target := strokesLabelled(see, "origin"), strokesLabelled(see, "target")
	if len(origin) != 1 || len(target) != 1 || origin[0].rgb != showBlue || target[0].rgb != showBlue {
		t.Fatalf("bodies %+v %+v, want one blue line each", origin, target)
	}
	oh := origin[0].at[5] - geoZ
	if o := origin[0].at; o[0] != spawnX || o[1] != spawnY || o[2] != geoZ || o[3] != spawnX || o[4] != spawnY || oh < 0 {
		t.Fatalf("origin body %v, want gm's from its feet up", o)
	}
	eyes := geoZ + oh*75/100
	los, limit := strokesLabelled(see, "Line-of-Sight"), strokesLabelled(see, "Geodata limit")
	if len(los) != 1 || los[0].rgb != showGreen || los[0].at != [6]int32{spawnX, spawnY, eyes, geoBeyondWallX, spawnY, eyes} {
		t.Fatalf("line of sight %+v, want green eye to eye at %d", los, eyes)
	}
	if len(limit) != 1 || limit[0].rgb != showMagenta || limit[0].at[2] != eyes+32 {
		t.Fatalf("geodata limit %+v, want magenta 32 above the eyes", limit)
	}
	var borders int
	for _, s := range see.strokes {
		if !s.line && s.rgb == showCyan {
			borders++
		}
	}
	// gm's cell to the selection's is 6 columns east, walked both ways.
	if borders != 12 {
		t.Fatalf("%d cyan cell borders, want 6 each way: %+v", borders, see.strokes)
	}

	frames = exchangeZones(t, gm, encodeBuildCmd("geo fly"))
	if len(frames) != 2 {
		t.Fatalf("//geo fly = %x, want the message and the drawing", opcodes(frames))
	}
	// Both stand on the ground: the corridor's floor meets it on the first
	// cell.
	assertTexts(t, frames[:1], "Can not fly beeline!")
	fly := decodePrimitive(t, frames[1])
	if fly.name != "CAN_FLY" {
		t.Fatalf("drawing %q, want CAN_FLY", fly.name)
	}
	low, high := int32(geoZ-32), geoZ+oh-32
	if l := strokesLabelled(fly, "origin"); len(l) != 1 || l[0].rgb != showBlue || l[0].at != [6]int32{spawnX, spawnY, low, spawnX, spawnY, high} {
		t.Fatalf("origin %+v, want gm's corridor blue 32 below", l)
	}
	for label, at := range map[string][6]int32{
		"Line-of-Flight MIN": {spawnX, spawnY, low, geoBeyondWallX, spawnY, low},
		"Line-of-Fligth MAX": {spawnX, spawnY, high, geoBeyondWallX, spawnY, high},
	} {
		if l := strokesLabelled(fly, label); len(l) != 1 || l[0].rgb != showRed || l[0].at != at {
			t.Fatalf("%s %+v, want red %v", label, l, at)
		}
	}
	stop := strokesLabelled(fly, "Can fly")
	if len(stop) != 1 || stop[0].rgb != showGreen {
		t.Fatalf("Can fly %+v, want one green line", stop)
	}
	sx, sy, sz := stop[0].at[3], stop[0].at[4], stop[0].at[5]
	if l := strokesLabelled(fly, "Inaccessible"); len(l) != 1 || l[0].rgb != showRed || l[0].at != [6]int32{sx, sy, sz, geoBeyondWallX, spawnY, low} {
		t.Fatalf("Inaccessible %+v, want red from the stop", l)
	}
	if l := strokesLabelled(fly, "Last position"); len(l) != 1 || l[0].at != [6]int32{sx, sy, sz, sx, sy, sz + oh} {
		t.Fatalf("Last position %+v, want the corridor at the stop", l)
	}
	white := stroke{line: true, rgb: showWhite, at: [6]int32{spawnX, spawnY, low, geoBeyondWallX, spawnY, low}}
	if !slices.Contains(fly.strokes, white) {
		t.Fatalf("CAN_FLY strokes %+v lack the white line to the selection", fly.strokes)
	}
}

// itoa is v in decimal.
func itoa[T ~int | ~int32](v T) string { return strconv.Itoa(int(v)) }
