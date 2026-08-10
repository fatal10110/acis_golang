package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestHostileBroadcastShotRechargeGivesObserversIndependentBuffers(t *testing.T) {
	hostile, receivers := newRetainedBroadcastFixture(t, 2)
	hostile.broadcastShotRecharge(123)
	assertIndependentFrames(t, receivers[0].frame, receivers[1].frame)
}

type retainedFrameReceiver struct {
	world.Presence
	id     int32
	frame  wire.Frame
	frames int
}

func (r *retainedFrameReceiver) ObjectID() int32 { return r.id }

func (r *retainedFrameReceiver) SendFrame(frame wire.Frame) bool {
	r.frame = frame
	r.frames++
	return true
}

func newRetainedBroadcastFixture(t testing.TB, observers int) (*Hostile, []*retainedFrameReceiver) {
	t.Helper()
	state := world.New()
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)
	receivers := make([]*retainedFrameReceiver, 0, observers)
	for i := 0; i < observers; i++ {
		receiver := &retainedFrameReceiver{id: int32(100 + i)}
		receivers = append(receivers, receiver)
		state.Spawn(receiver, 100+i, 0, 0, 0)
	}
	return hostile, receivers
}

func assertIndependentFrames(t testing.TB, first, second wire.Frame) {
	t.Helper()
	defer first.Release()
	defer second.Release()
	if len(first.Bytes()) <= wire.FrameHeaderSize || len(second.Bytes()) <= wire.FrameHeaderSize {
		t.Fatal("observers did not receive frames")
	}
	secondPayload := second.Bytes()[wire.FrameHeaderSize]
	first.Bytes()[wire.FrameHeaderSize] ^= 0xff
	if second.Bytes()[wire.FrameHeaderSize] != secondPayload {
		t.Fatal("mutating one observer frame changed another observer frame")
	}
}
