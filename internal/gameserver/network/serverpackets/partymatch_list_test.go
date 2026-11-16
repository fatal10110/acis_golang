package serverpackets

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestPartyMatchListFitsOneFrame pins the room list staying one sendable
// frame however many rooms exist (#5101, owner-approved): rows past the
// frame limit are left off, and the count and flag describe the rows
// sent.
func TestPartyMatchListFitsOneFrame(t *testing.T) {
	row := func(id int32) PartyRoom {
		return PartyRoom{
			ID: id, Title: strings.Repeat("t", 64), Location: 7, MinLevel: 1, MaxLevel: 80,
			Members: 1, MaxMembers: 12, LeaderName: strings.Repeat("n", 16),
		}
	}
	const rowSize = 6*4 + 2*65 + 2*17
	const head = wire.FrameHeaderSize + 1 + 4 + 4
	fit := (wire.MaxFrameLength - head) / rowSize

	for _, n := range []int{0, 1, fit, fit + 1, 1000} {
		rooms := make([]PartyRoom, n)
		for i := range rooms {
			rooms[i] = row(int32(i + 1))
		}
		frame := FramePartyMatchList(rooms)
		if err := frame.Err(); err != nil {
			t.Fatalf("%d rooms: frame error %v", n, err)
		}
		payload := framePayload(t, frame)
		want := min(n, fit)
		if len(payload)+wire.FrameHeaderSize != head+want*rowSize {
			t.Fatalf("%d rooms: frame length %d, want %d", n, len(payload)+wire.FrameHeaderSize, head+want*rowSize)
		}
		r := wire.NewReader(payload[1:])
		if flag, count := r.ReadInt32(), r.ReadInt32(); flag != boolInt32(want > 0) || count != int32(want) {
			t.Fatalf("%d rooms: flag %d count %d, want %d rooms", n, flag, count, want)
		}
		for i := range want {
			if id := r.ReadInt32(); id != int32(i+1) {
				t.Fatalf("%d rooms: row %d id %d", n, i, id)
			}
			r.ReadString()
			for range 5 {
				r.ReadInt32()
			}
			r.ReadString()
		}
		if err := r.Err(); err != nil || r.Remaining() != 0 {
			t.Fatalf("%d rooms: err %v, %d bytes left", n, err, r.Remaining())
		}
	}
}
