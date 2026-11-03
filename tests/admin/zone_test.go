package admin

import (
	"encoding/binary"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const zoneShowInvalid = "Invalid parameter for //zone show."

// primitive is one decoded ExServerPrimitive frame.
type primitive struct {
	name    string
	anchor  [3]int32
	strokes []stroke
}

// stroke is one point (line false) or line of a primitive frame.
type stroke struct {
	line    bool
	label   string
	rgb     [3]int32
	colored bool
	at      [6]int32
}

// decodePrimitive decodes frame as ExServerPrimitive.
func decodePrimitive(t *testing.T, frame []byte) primitive {
	t.Helper()
	if len(frame) < 3 || frame[0] != serverpackets.OpcodeExtended || binary.LittleEndian.Uint16(frame[1:]) != serverpackets.OpcodeExServerPrimitive {
		t.Fatalf("frame %x is not ExServerPrimitive", frame)
	}
	b := frame[3:]
	str := func() string {
		var units []uint16
		for {
			u := binary.LittleEndian.Uint16(b)
			b = b[2:]
			if u == 0 {
				return string(utf16.Decode(units))
			}
			units = append(units, u)
		}
	}
	d := func() int32 {
		v := int32(binary.LittleEndian.Uint32(b))
		b = b[4:]
		return v
	}
	var p primitive
	p.name = str()
	p.anchor = [3]int32{d(), d(), d()}
	if r, a := d(), d(); r != 0x7fffffff || a != 0x7fffffff {
		t.Fatalf("%s range, angle = %d, %d, want max int", p.name, r, a)
	}
	for n := d(); n > 0; n-- {
		kind := b[0]
		b = b[1:]
		s := stroke{line: kind == 2, label: str()}
		s.rgb = [3]int32{d(), d(), d()}
		s.colored = d() == 1
		coords := 3
		if s.line {
			coords = 6
		}
		for i := range coords {
			s.at[i] = d()
		}
		p.strokes = append(p.strokes, s)
	}
	if len(b) != 0 {
		t.Fatalf("%s: %d trailing bytes", p.name, len(b))
	}
	return p
}

// zoneTestIndex holds, around the spawn at (10, 20): peace zone 1 under
// it, arena 2 one region east, peace 3 three regions east (out of reach of
// show all), and the wide arena 4 far off whose outline takes two frames.
func zoneTestIndex(t *testing.T) *zone.Index {
	t.Helper()
	must := func(f zone.Form, err error) zone.Form {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	ix := zone.NewIndex()
	ix.Add(zone.NewPeace(1, must(zone.NewCuboid(-100, 100, -100, 100, -200, 200))))
	ix.Add(zone.NewArena(2, must(zone.NewCylinder(3000, 20, -100, 100, 100))))
	ix.Add(zone.NewPeace(3, must(zone.NewCuboid(7000, 7200, 0, 200, -200, 200))))
	ix.Add(zone.NewArena(4, must(zone.NewCylinder(40000, 40000, -500, 500, 1000))))
	return ix
}

// zoneTestRestarts is a restart point covering the spawn's map region.
func zoneTestRestarts() *restart.Table {
	return &restart.Table{Points: []restart.Point{{
		Name: "TestTown",
		MapRegions: []location.Point{{
			X: (spawnX-world.MinX)/world.TileSize + world.TileXMin,
			Y: (spawnY-world.MinY)/world.TileSize + world.TileYMin,
		}},
	}}}
}

func bootZoneAdmin(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient) {
	t.Helper()
	opts = append([]gameservertest.Option{
		gameservertest.WithZones(zoneTestIndex(t)),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "zone.htm")),
	}, opts...)
	srv, _ := bootAdmin(t, adminLevel, opts...)
	enterWorld(t, srv.Client)
	return srv, srv.Client
}

// assertBlank checks p is the frame name blanked: only the white point at
// the spawn, high above the world.
func assertBlank(t *testing.T, p primitive, name string) {
	t.Helper()
	want := stroke{rgb: [3]int32{255, 255, 255}, at: [6]int32{spawnX, spawnY, 16384}}
	if p.name != name || p.anchor != [3]int32{spawnX, spawnY, -16000} || len(p.strokes) != 1 || p.strokes[0] != want {
		t.Fatalf("frame = %+v, want %s blank at the spawn", p, name)
	}
}

// TestAdminZoneShowAll pins //zone show all: every zone of the game
// master's region and the regions around it is outlined in one drawing
// named ZONE anchored at the spawn 16000 below sea level, points before
// lines, then the zone panel opens on what the game master stands in.
func TestAdminZoneShowAll(t *testing.T) {
	t.Parallel()
	srv, gm := bootZoneAdmin(t, gameservertest.WithRestartPoints(zoneTestRestarts()))

	frames := exchangeZones(t, gm, encodeBuildCmd("zone show all"))
	if len(frames) != 2 || frames[1][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("//zone show all = %x, want ExServerPrimitive then the panel", opcodes(frames))
	}
	p := decodePrimitive(t, frames[0])
	if p.name != "ZONE" || p.anchor != [3]int32{spawnX, spawnY, -16000} {
		t.Fatalf("drawing %q at %v, want ZONE at the spawn 16000 below", p.name, p.anchor)
	}
	// Arena 2 is 12 posts (radius 100, every 50 around): a floor and a
	// ceiling point each come first; then peace 1's 12 lines; then the 12
	// posts' edges.
	if len(p.strokes) != 12*3+12 {
		t.Fatalf("strokes = %d, want 48", len(p.strokes))
	}
	green, red, yellow := [3]int32{0, 255, 0}, [3]int32{255, 0, 0}, [3]int32{255, 255, 0}
	wantFirst := []stroke{
		{label: "ArenaZone[2]", rgb: green, colored: true, at: [6]int32{3100, 20, -132}},
		{label: "ArenaZone[2]", rgb: red, colored: true, at: [6]int32{3100, 20, 68}},
	}
	for i, w := range wantFirst {
		if p.strokes[i] != w {
			t.Fatalf("stroke %d = %+v, want %+v", i, p.strokes[i], w)
		}
	}
	// The box's floor and ceiling collapse onto its near x side.
	peace := []stroke{
		{line: true, label: "PeaceZone[1]", rgb: green, colored: true, at: [6]int32{-100, -100, -232, -100, 100, -232}},
		{line: true, label: "PeaceZone[1]", rgb: green, colored: true, at: [6]int32{-100, 100, -232, -100, 100, -232}},
		{line: true, label: "PeaceZone[1]", rgb: green, colored: true, at: [6]int32{-100, 100, -232, -100, -100, -232}},
		{line: true, label: "PeaceZone[1]", rgb: green, colored: true, at: [6]int32{-100, -100, -232, -100, -100, -232}},
		{line: true, label: "PeaceZone[1]", rgb: red, colored: true, at: [6]int32{-100, -100, 168, -100, 100, 168}},
		{line: true, label: "PeaceZone[1]", rgb: red, colored: true, at: [6]int32{-100, 100, 168, -100, 100, 168}},
		{line: true, label: "PeaceZone[1]", rgb: red, colored: true, at: [6]int32{-100, 100, 168, -100, -100, 168}},
		{line: true, label: "PeaceZone[1]", rgb: red, colored: true, at: [6]int32{-100, -100, 168, -100, -100, 168}},
		{line: true, rgb: yellow, colored: true, at: [6]int32{-100, -100, -232, -100, -100, 168}},
		{line: true, rgb: yellow, colored: true, at: [6]int32{-100, 100, -232, -100, 100, 168}},
		{line: true, rgb: yellow, colored: true, at: [6]int32{100, -100, -232, 100, -100, 168}},
		{line: true, rgb: yellow, colored: true, at: [6]int32{100, 100, -232, 100, 100, 168}},
	}
	for i, w := range peace {
		if got := p.strokes[24+i]; got != w {
			t.Fatalf("peace stroke %d = %+v, want %+v", i, got, w)
		}
	}
	if got, want := p.strokes[36], (stroke{line: true, rgb: yellow, colored: true, at: [6]int32{3100, 20, -132, 3100, 20, 68}}); got != want {
		t.Fatalf("first arena edge = %+v, want %+v", got, want)
	}

	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	page := htmlBody(t, frames[1])
	for _, want := range []string{
		"<td align=right width=140>1 </td>",
		"<td align=right width=140>" + strconv.Itoa((x-world.MinX)/world.TileSize+world.TileXMin) + "_" + strconv.Itoa((y-world.MinY)/world.TileSize+world.TileYMin) + "</td>",
		"<td align=right width=140>false</td>",
		"<td align=right width=140>" + strconv.Itoa(x) + ", " + strconv.Itoa(y) + ", " + strconv.Itoa(z) + "</td>",
		"<font color=\"LEVEL\">PEACE<br1></font>",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("panel misses %q:\n%s", want, page)
		}
	}
	// The calculated and the current restart point.
	if n := strings.Count(page, ">TestTown</td>"); n != 2 {
		t.Fatalf("panel names TestTown %d times, want 2:\n%s", n, page)
	}
}

// TestAdminZoneShowID pins //zone show <id>: that zone alone, without the
// panel. An outline past 16000 bytes continues in a frame named ZONE1;
// show clear then blanks both, and the next clear blanks only ZONE.
func TestAdminZoneShowID(t *testing.T) {
	t.Parallel()
	_, gm := bootZoneAdmin(t, gameservertest.WithRestartPoints(zoneTestRestarts()))

	frames := exchangeZones(t, gm, encodeBuildCmd("zone show 4"))
	if len(frames) != 2 {
		t.Fatalf("//zone show 4 = %x, want two drawing frames", opcodes(frames))
	}
	first, second := decodePrimitive(t, frames[0]), decodePrimitive(t, frames[1])
	if first.name != "ZONE" || second.name != "ZONE1" || second.anchor != first.anchor {
		t.Fatalf("frames %q, %q, want ZONE then ZONE1 at one anchor", first.name, second.name)
	}
	// 125 posts of two points and an edge.
	if n := len(first.strokes) + len(second.strokes); n != 125*3 {
		t.Fatalf("strokes = %d, want 375", n)
	}
	for _, s := range append(first.strokes, second.strokes...) {
		if s.label != "ArenaZone[4]" && s.label != "" {
			t.Fatalf("stroke %+v, want arena 4's", s)
		}
	}

	frames = exchangeZones(t, gm, encodeBypass("admin_zone show clear"))
	if len(frames) != 3 || frames[2][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("//zone show clear = %x, want ZONE, ZONE1, the panel", opcodes(frames))
	}
	assertBlank(t, decodePrimitive(t, frames[0]), "ZONE")
	assertBlank(t, decodePrimitive(t, frames[1]), "ZONE1")

	frames = exchangeZones(t, gm, encodeBuildCmd("zone SHOW Clear"))
	if len(frames) != 2 || frames[1][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("second //zone show clear = %x, want ZONE then the panel", opcodes(frames))
	}
	assertBlank(t, decodePrimitive(t, frames[0]), "ZONE")
}

// TestAdminZoneRefusals pins what //zone answers besides a drawing: a show
// without a known zone id or keyword is refused, a bare //zone and any
// other argument (//zone clear among them) open the panel alone, and
// without a restart point where the game master stands the panel is not
// shown: the bare command fails the action, a show is refused after its
// drawing.
func TestAdminZoneRefusals(t *testing.T) {
	t.Parallel()
	t.Run("restart point", func(t *testing.T) {
		t.Parallel()
		_, gm := bootZoneAdmin(t, gameservertest.WithRestartPoints(zoneTestRestarts()))
		for _, cmd := range []string{"zone show", "zone show 99", "zone show abc", "zone show 2147483648", "zone show -1"} {
			assertTexts(t, exchangeZones(t, gm, encodeBuildCmd(cmd)), zoneShowInvalid)
		}
		for _, cmd := range []string{"zone", "zone clear", "zone visualize"} {
			frames := exchangeZones(t, gm, encodeBuildCmd(cmd))
			if len(frames) != 1 || !strings.Contains(htmlBody(t, frames[0]), "Zone Check") {
				t.Fatalf("//%s = %x, want the panel alone", cmd, opcodes(frames))
			}
		}
		// A show id may carry a sign.
		frames := exchangeZones(t, gm, encodeBuildCmd("zone show +1"))
		if len(frames) != 1 || decodePrimitive(t, frames[0]).strokes[0].label != "PeaceZone[1]" {
			t.Fatalf("//zone show +1 = %x, want peace 1 drawn", opcodes(frames))
		}
	})
	t.Run("no restart point", func(t *testing.T) {
		t.Parallel()
		_, gm := bootZoneAdmin(t)
		frames := exchangeZones(t, gm, encodeBuildCmd("zone"))
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("//zone = %x, want ActionFailed", opcodes(frames))
		}
		frames = exchangeZones(t, gm, encodeBuildCmd("zone show clear"))
		if len(frames) != 2 {
			t.Fatalf("//zone show clear = %x, want the drawing then a refusal", opcodes(frames))
		}
		assertBlank(t, decodePrimitive(t, frames[0]), "ZONE")
		assertTexts(t, frames[1:], zoneShowInvalid)
	})
}
