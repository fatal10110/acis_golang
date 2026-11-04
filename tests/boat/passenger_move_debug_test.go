package boat

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"sync/atomic"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// utf16Z is s as a null-terminated UTF-16LE packet string.
func utf16Z(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return append(b, 0, 0)
}

// TestPassengerDeckReportDrawnWhenDebugged pins //show move on a
// passenger: a deck position report is drawn on MOVE<id> as a green point
// at the reported position, labelled with the 500 drift allowance and the
// drift from the server's deck position: "speed=500.0 desync=5.0".
func TestPassengerDeckReportDrawnWhenDebugged(t *testing.T) {
	t.Parallel()
	srv, c, me := bootReload(t, nil, &atomic.Bool{}, passengerItinerary())
	b := srv.Boats.Boats()[0].ObjectID()
	c.Send(encodeAction(me, false))
	srv.ReadQueued(t, c)
	c.Send(encodeBuildCmd("show move"))
	srv.ReadQueued(t, c)
	board(t, srv, c, b)
	stop := location.Location{X: 50, Y: -150, Z: -40}
	c.Send(encodeCannotMoveInVehicle(b, stop, 16384))
	srv.ReadQueued(t, c)

	reported := location.Location{X: 53, Y: -146, Z: -40}
	c.Send(encodeValidatePosition(reported, 0, b))
	var drawn [][]byte
	for _, f := range srv.ReadQueued(t, c) {
		if len(f) >= 3 && f[0] == serverpackets.OpcodeExtended && binary.LittleEndian.Uint16(f[1:]) == serverpackets.OpcodeExServerPrimitive {
			drawn = append(drawn, f)
		}
	}
	if len(drawn) != 1 || !bytes.HasPrefix(drawn[0][3:], utf16Z("MOVE"+strconv.Itoa(int(me)))) {
		t.Fatalf("drawings on the deck report = %x, want MOVE%d alone", drawn, me)
	}
	// The point: its label, green, the label colored, then the reported
	// position.
	var want []byte
	want = append(want, 1)
	want = append(want, utf16Z("speed=500.0 desync=5.0")...)
	for _, v := range []int32{0, 255, 0, 1, int32(reported.X), int32(reported.Y), int32(reported.Z)} {
		want = binary.LittleEndian.AppendUint32(want, uint32(v))
	}
	if !bytes.Contains(drawn[0], want) {
		t.Fatalf("MOVE%d = %x, want the green deck report point %x", me, drawn[0], want)
	}
}
