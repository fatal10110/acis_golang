package admin

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// moviePages are the shipped movie maker pages.
var moviePages = []string{"movie/main_empty.htm", "movie/main_notempty.htm", "movie/add_sequence.htm", "movie/edit_sequence.htm"}

// specialCamera returns a SpecialCamera frame's ten fields.
func specialCamera(t *testing.T, frame []byte) [10]int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSpecialCamera {
		t.Fatalf("opcode = %#x, want SpecialCamera", frame[0])
	}
	r := wire.NewReader(frame[1:])
	var v [10]int32
	for i := range v {
		v[i] = r.ReadInt32()
	}
	return v
}

// assertCameras requires frames to be exactly the SpecialCamera shots want.
func assertCameras(t *testing.T, what string, frames [][]byte, want ...[10]int32) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("%s frames = %x, want %d SpecialCamera", what, testsupport.FrameOpcodes(frames), len(want))
	}
	for i, f := range frames {
		if got := specialCamera(t, f); got != want[i] {
			t.Fatalf("%s shot %d = %v, want %v", what, i, got, want[i])
		}
	}
}

// sequenceRow is one sequence row of AdminMovieMaker.mainHtm.
func sequenceRow(values ...int) string {
	s := "<tr>"
	for _, v := range values {
		s += "<td>" + strconv.Itoa(v) + "</td>"
	}
	return s + "</tr>"
}

// TestAdminMovie pins AdminMovieMaker.java: //sequence add records a shot
// around the GM's target, or the GM, under an id that keeps its first
// shot; the panel lists the shots in id order; //sequence play and
// broadcast show one shot at once, //movie play and broadcast every shot,
// the first after 500 ms and each next one 100 ms before the one before
// ends; each refusal names itself and reopens the panel.
func TestAdminMovie(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, moviePages...)))
	gm := srv.Client
	enterWorld(t, gm)
	other, otherID := addPlayer(t, srv, "other", "Other", userLevel)
	drain(t, gm)
	drain(t, other)

	empty := shippedPage(t, "movie/main_empty.htm")
	assertOnePage(t, "//movie", exchange(t, gm, encodeBuildCmd("movie")), empty)
	assertOnePage(t, "//sequence", exchange(t, gm, encodeBuildCmd("sequence")), empty)
	frames := exchange(t, gm, encodeBuildCmd("movie play"))
	assertTexts(t, frames[:1], "There is nothing to play.")
	assertOnePage(t, "//movie play", frames[1:], empty)
	for _, cmd := range []string{"movie stop", "sequence stop"} {
		if frames := exchange(t, gm, encodeBuildCmd(cmd)); len(frames) != 0 {
			t.Fatalf("//%s frames = %x, want none", cmd, testsupport.FrameOpcodes(frames))
		}
	}
	assertOnePage(t, "//sequence add", exchange(t, gm, encodeBypass("admin_sequence add")), shippedPage(t, "movie/add_sequence.htm"))

	for _, c := range []struct{ cmd, text string }{
		{"sequence add -1", "The sequence id is invalid."},
		{"sequence add 1", "The sequence couldn't be updated."},
		{"sequence add 1 100 0 0 0 1000 0 0", "Some arguments are missing."},
		{"sequence add 1 100 0 0 0 1000 0 0 1 9", "Some arguments are missing."},
		{"sequence delete 1", "The sequence id doesn't exist."},
		{"sequence delete x", "You entered an invalid sequence id."},
		{"sequence play 1", "The sequence id doesn't exist."},
		{"sequence play", "You entered an invalid sequence id."},
		{"sequence broadcast x", "You entered an invalid sequence id."},
	} {
		frames := exchange(t, gm, encodeBuildCmd(c.cmd))
		if len(frames) != 2 {
			t.Fatalf("//%s frames = %x, want the refusal and the panel", c.cmd, testsupport.FrameOpcodes(frames))
		}
		assertTexts(t, frames[:1], c.text)
		assertOnePage(t, "//"+c.cmd, frames[1:], empty)
	}
	assertOnePage(t, "//sequence add x", exchange(t, gm, encodeBuildCmd("sequence add x")), empty)
	assertOnePage(t, "//sequence add 1 x 0 0 0 1000 0 0 1", exchange(t, gm, encodeBuildCmd("sequence add 1 x 0 0 0 1000 0 0 1")), empty)

	// Shot 2 is recorded around the GM, shot 1 around the selected Other.
	two := shippedPage(t, "movie/main_notempty.htm", "%sequences%", sequenceRow(2, 300, 10, 20, 500, 3000, 5, 6, 0))
	assertOnePage(t, "//sequence add 2", exchange(t, gm, encodeBuildCmd("sequence add 2 300 10 20 500 3000 5 6 0")), two)
	assertOnePage(t, "//sequence add 2 again", exchange(t, gm, encodeBuildCmd("sequence add 2 1 1 1 1 1 1 1 1")), two)
	exchange(t, gm, encodeAction(otherID))
	settle(t, other)
	both := shippedPage(t, "movie/main_notempty.htm", "%sequences%",
		sequenceRow(1, 100, -30, 40, 200, 2000, 0, 0, 1)+sequenceRow(2, 300, 10, 20, 500, 3000, 5, 6, 0))
	assertOnePage(t, "//sequence add 1", exchange(t, gm, encodeBypass("admin_sequence add 1 100 -30 40 200 2000 0 0 1")), both)
	assertOnePage(t, "//sequence add 2 (edit)", exchange(t, gm, encodeBypass("admin_sequence add 2")), shippedPage(t, "movie/edit_sequence.htm",
		"%sId%", "2", "%sDist%", "300", "%sYaw%", "10", "%sPitch%", "20", "%sTime%", "500",
		"%sDuration%", "3000", "%sTurn%", "5", "%sRise%", "6", "%sWidescreen%", "0"))

	shot1 := [10]int32{otherID, 100, -30, 40, 200, 2000, 0, 0, 1, 0}
	shot2 := [10]int32{gmID, 300, 10, 20, 500, 3000, 5, 6, 0, 0}
	assertCameras(t, "//sequence play 2", exchange(t, gm, encodeBuildCmd("sequence play 2")), shot2)
	if frames := settle(t, other); len(frames) != 0 {
		t.Fatalf("Other frames after //sequence play = %x, want none", testsupport.FrameOpcodes(frames))
	}
	assertCameras(t, "//sequence broadcast 1", exchange(t, gm, encodeBuildCmd("sequence broadcast 1")), shot1)
	assertCameras(t, "Other after //sequence broadcast 1", settle(t, other), shot1)

	// //movie play: shot 1 at 500 ms, shot 2 at 500 + 2000 - 100 ms.
	if frames := exchange(t, gm, encodeBuildCmd("movie play")); len(frames) != 0 {
		t.Fatalf("//movie play frames = %x, want none at once", testsupport.FrameOpcodes(frames))
	}
	srv.Advance(t, 499*time.Millisecond)
	assertCameras(t, "GM at 499 ms", settle(t, gm))
	srv.Advance(t, time.Millisecond)
	assertCameras(t, "GM at 500 ms", settle(t, gm), shot1)
	srv.Advance(t, 1899*time.Millisecond)
	assertCameras(t, "GM at 2399 ms", settle(t, gm))
	srv.Advance(t, time.Millisecond)
	assertCameras(t, "GM at 2400 ms", settle(t, gm), shot2)
	assertCameras(t, "Other after //movie play", settle(t, other))

	if frames := exchange(t, gm, encodeBuildCmd("movie broadcast")); len(frames) != 0 {
		t.Fatalf("//movie broadcast frames = %x, want none at once", testsupport.FrameOpcodes(frames))
	}
	srv.Advance(t, 2400*time.Millisecond)
	gmFrames, otherFrames := settle(t, gm), settle(t, other)
	assertCameras(t, "GM after //movie broadcast", gmFrames, shot1, shot2)
	assertCameras(t, "Other after //movie broadcast", otherFrames, shot1, shot2)
	// Each recipient got its own frame: changing one leaves the other.
	before := slices.Clone(otherFrames[0])
	gmFrames[0][1] ^= 0xff
	if !slices.Equal(otherFrames[0], before) {
		t.Fatal("GM and Other share one SpecialCamera frame")
	}

	frames = exchange(t, gm, encodeBuildCmd("sequence delete 1"))
	assertOnePage(t, "//sequence delete 1", frames, shippedPage(t, "movie/main_notempty.htm", "%sequences%", sequenceRow(2, 300, 10, 20, 500, 3000, 5, 6, 0)))
}
