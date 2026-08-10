package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type sendFrameReceiver struct {
	world.Presence
	id     int32
	frame  wire.Frame
	frames int
}

func (r *sendFrameReceiver) ObjectID() int32 { return r.id }

func (r *sendFrameReceiver) SendFrame(frame wire.Frame) bool {
	r.frame = frame
	r.frames++
	return true
}

type nonReceiver struct {
	world.Presence
	id int32
}

func (n *nonReceiver) ObjectID() int32 { return n.id }

func TestSendFrameBuildsOnceForFrameCapableReceivers(t *testing.T) {
	receivers := []world.Tracked{&sendFrameReceiver{id: 1}, &sendFrameReceiver{id: 2}}
	builds := 0
	sendFrame(receivers, func() wire.Frame {
		builds++
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
	if builds != 1 {
		t.Fatalf("frame builds = %d, want 1", builds)
	}
	for _, r := range receivers {
		receiver := r.(*sendFrameReceiver)
		defer receiver.frame.Release()
		if receiver.frames != 1 {
			t.Fatalf("receiver %d frames = %d, want 1", receiver.id, receiver.frames)
		}
	}
}

func TestSendFrameSkipsBuildWithoutFrameCapableReceivers(t *testing.T) {
	receivers := []world.Tracked{&nonReceiver{id: 1}}
	builds := 0
	sendFrame(receivers, func() wire.Frame {
		builds++
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
	if builds != 0 {
		t.Fatalf("frame builds = %d, want 0", builds)
	}
}

func TestSendFrameGivesReceiversIndependentBuffers(t *testing.T) {
	first := &sendFrameReceiver{id: 1}
	second := &sendFrameReceiver{id: 2}
	sendFrame([]world.Tracked{first, second}, func() wire.Frame {
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
	defer first.frame.Release()
	defer second.frame.Release()
	if len(first.frame.Bytes()) <= wire.FrameHeaderSize || len(second.frame.Bytes()) <= wire.FrameHeaderSize {
		t.Fatal("receivers did not receive frames")
	}
	secondPayload := second.frame.Bytes()[wire.FrameHeaderSize]
	first.frame.Bytes()[wire.FrameHeaderSize] ^= 0xff
	if second.frame.Bytes()[wire.FrameHeaderSize] != secondPayload {
		t.Fatal("mutating one receiver's frame changed another receiver's frame")
	}
}
