package pets

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

func onQueue(t *testing.T, q *sim.Queue, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !q.Post(func() { fn(); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// TestBluffOnServitorShowsTurn pins #4928 on a servitor: Bluff landing on
// it shows its owner a full-speed spin from the servitor's heading (side 1,
// speed 65535), then at once a stop on the heading of the monster that cast
// it (EffectBluff.onStart), and leaves the servitor facing that way.
func TestBluffOnServitorShowsTurn(t *testing.T) {
	t.Parallel()
	h, cat, hostile := bootFearServitor(t, catTemplate(), summonCatSkillID)
	const from, to = 20000, 40000
	onQueue(t, cat.Queue(), func() { cat.SetHeading(from) })
	onQueue(t, hostile.Queue(), func() { hostile.SetHeading(to) })
	drainUntilQuiet(t, h.client)

	landFearPetEffect(t, cat, hostile, "Bluff", 358, 1, 3)

	frames := h.client.ReadQueued()
	le := binary.LittleEndian
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeStartRotation {
			continue
		}
		if got := [4]int32{int32(le.Uint32(f[1:])), int32(le.Uint32(f[5:])), int32(le.Uint32(f[9:])), int32(le.Uint32(f[13:]))}; got != [4]int32{cat.ObjectID(), from, 1, 65535} {
			t.Fatalf("StartRotation object/degree/side/speed = %v, want [%d %d 1 65535]", got, cat.ObjectID(), from)
		}
		if i+1 == len(frames) || frames[i+1][0] != serverpackets.OpcodeStopRotation {
			t.Fatal("StartRotation not followed at once by StopRotation")
		}
		stop := frames[i+1]
		if got := [3]int32{int32(le.Uint32(stop[1:])), int32(le.Uint32(stop[5:])), int32(le.Uint32(stop[9:]))}; got != [3]int32{cat.ObjectID(), to, 65535} {
			t.Fatalf("StopRotation object/degree/speed = %v, want [%d %d 65535]", got, cat.ObjectID(), to)
		}
		if got := cat.Heading(); got != to {
			t.Fatalf("servitor heading = %d after Bluff, want %d", got, to)
		}
		return
	}
	t.Fatalf("owner saw no StartRotation in %d frames", len(frames))
}
