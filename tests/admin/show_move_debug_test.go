package admin

import (
	"context"
	"encoding/binary"
	"regexp"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// addDebugViewer enters the world at the game master's spawn on wallGeo a
// player of access level, so it knows the game master.
func addDebugViewer(t *testing.T, srv *gameservertest.Server, account, name string, level int) *testsupport.ScriptedClient {
	t.Helper()
	ch := srv.SeedCharacterFor(t, account, name, 1, 0)
	setAccessLevel(t, srv, ch.ID, level)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", spawnX, spawnY, geoZ, ch.ID); err != nil {
		t.Fatal(err)
	}
	c := srv.DialClient(t, account, 1)
	enterWorld(t, c)
	return c
}

func encodeReportedPosition(x, y, z int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(0) // heading
	w.WriteInt32(0) // boat id
	return w.Bytes()
}

func isPrimitive(frame []byte) bool {
	return len(frame) >= 3 && frame[0] == serverpackets.OpcodeExtended && binary.LittleEndian.Uint16(frame[1:]) == serverpackets.OpcodeExServerPrimitive
}

// debugFrames splits frames into the drawings and the texts among them,
// in order.
func debugFrames(t *testing.T, frames [][]byte) ([]primitive, []string) {
	t.Helper()
	var drawings []primitive
	var texts []string
	for _, f := range frames {
		switch {
		case isPrimitive(f):
			drawings = append(drawings, decodePrimitive(t, f))
		case f[0] == serverpackets.OpcodeSystemMessage:
			if id, text := systemText(t, f); id == serverpackets.SystemMessageS1 {
				texts = append(texts, text)
			}
		}
	}
	return drawings, texts
}

// TestAdminShowMoveAndPathDebugDrawWalk pins the drawings of a debugged
// walk: routed around wallGeo's wall, the search is drawn on PATH<id> and
// sent at once; the walk is told as "Moving from <x, y, z> to <x, y, z>"
// and drawn on MOVE<id> (the click white, the start a red point, the route
// yellow), sent with the first step, each step a red point labelled with
// the heading; a reported position adds a green point labelled with the
// speed and desync. Every game master that knows the walker, the walker
// itself and nobody else gets them.
func TestAdminShowMoveAndPathDebugDrawWalk(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootGeoAdmin(t)
	gmID := srv.SoleObjectID(t)
	watcher := addDebugViewer(t, srv, "dbgwatch", "Watcher", adminLevel)
	bystander := addDebugViewer(t, srv, "dbgbystand", "Bystander", userLevel)
	drain(t, gm)
	gm.Send(encodeAction(gmID))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show move")), "Debug move enabled on Admin")
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show path")), "Debug path enabled on Admin")
	drain(t, watcher)
	drain(t, bystander)

	moveName, pathName := "MOVE"+strconv.Itoa(int(gmID)), "PATH"+strconv.Itoa(int(gmID))
	moving := regexp.MustCompile(`^Moving from ` + itoa(spawnX) + `, ` + itoa(spawnY) + `, ` + itoa(geoZ) + ` to (-?\d+), (-?\d+), (-?\d+)$`)
	timing := regexp.MustCompile(`^\d+ms$`)
	for _, c := range []*testsupport.ScriptedClient{gm, watcher} {
		var frames [][]byte
		if c == gm {
			frames = exchangeZones(t, gm, encodeMove(geoBeyondWallX, spawnY, geoZ))
		} else {
			frames = exchangeZones(t, c, nil)
		}
		drawings, texts := debugFrames(t, frames)
		if len(drawings) != 1 || drawings[0].name != pathName {
			t.Fatalf("drawings on the walk = %+v, want %s alone", drawings, pathName)
		}
		timed := false
		for _, s := range drawings[0].strokes {
			timed = timed || (!s.line && s.rgb == showRed && timing.MatchString(s.label))
		}
		if !timed {
			t.Fatalf("%s strokes %+v lack the red search time", pathName, drawings[0].strokes)
		}
		if len(texts) != 1 || !moving.MatchString(texts[0]) {
			t.Fatalf("texts on the walk = %q, want %q", texts, moving)
		}
	}
	if drawings, texts := debugFrames(t, exchangeZones(t, bystander, nil)); len(drawings) != 0 || len(texts) != 0 {
		t.Fatalf("bystander got %+v %q, want no debug", drawings, texts)
	}

	srv.TickPositions()
	for _, c := range []*testsupport.ScriptedClient{gm, watcher} {
		drawings, _ := debugFrames(t, exchangeZones(t, c, nil))
		if len(drawings) == 0 || drawings[0].name != moveName {
			t.Fatalf("drawings on the first steps = %+v, want %s", drawings, moveName)
		}
		labels := map[string]int{}
		for _, s := range drawings[0].strokes {
			switch {
			case s.line && s.rgb == showWhite && regexp.MustCompile(`^MoveToLocation: -?\d+, -?\d+, -?\d+$`).MatchString(s.label):
				labels["click"]++
				if [3]int32(s.at[:3]) != [3]int32{spawnX, spawnY, geoZ} {
					t.Fatalf("click line %+v, want it from the start", s)
				}
			case !s.line && s.rgb == showRed && s.label == "" && [3]int32(s.at[:3]) == [3]int32{spawnX, spawnY, geoZ}:
				labels["start"]++
			case s.line && s.rgb == showYellow && s.label == "Segment #1":
				labels["route"]++
			case !s.line && s.rgb == showRed && regexp.MustCompile(`^\d+$`).MatchString(s.label):
				labels["step"]++
			}
		}
		if labels["click"] != 1 || labels["start"] != 1 || labels["route"] != 1 || labels["step"] != 1 {
			t.Fatalf("%s strokes %+v, want the click, the start, the route and one step", moveName, drawings[0].strokes)
		}
	}

	frames := exchangeZones(t, gm, encodeReportedPosition(spawnX, spawnY, geoZ))
	drawings, _ := debugFrames(t, frames)
	if len(drawings) != 1 || drawings[0].name != moveName {
		t.Fatalf("drawings on the report = %+v, want %s", drawings, moveName)
	}
	desync := regexp.MustCompile(`^speed=\d+\.\d+ desync=\d+\.\d+(E-?\d+)?$`)
	reported := 0
	for _, s := range drawings[0].strokes {
		if !s.line && s.rgb == showGreen && desync.MatchString(s.label) && [3]int32(s.at[:3]) == [3]int32{spawnX, spawnY, geoZ} {
			reported++
		}
	}
	if reported != 1 {
		t.Fatalf("%s strokes %+v, want the reported position green", moveName, drawings[0].strokes)
	}
}
