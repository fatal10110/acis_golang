package npc

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestHostileBroadcastFrameBuildsOnceForKnownObservers(t *testing.T) {
	hostile, receivers := newRetainedBroadcastFixture(t, 2)
	builds := 0
	hostile.broadcastFrame(func() wire.Frame {
		builds++
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
	if builds != 1 {
		t.Fatalf("frame builds = %d, want 1", builds)
	}
	for _, receiver := range receivers {
		defer receiver.frame.Release()
		if receiver.frames != 1 {
			t.Fatalf("receiver %d frames = %d, want 1", receiver.id, receiver.frames)
		}
	}
}

func TestHostileBroadcastFrameSkipsBuildWithoutObservers(t *testing.T) {
	hostile, _ := newRetainedBroadcastFixture(t, 0)
	builds := 0
	hostile.broadcastFrame(func() wire.Frame {
		builds++
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
	if builds != 0 {
		t.Fatalf("frame builds = %d, want 0", builds)
	}
}

func TestHostileBroadcastFrameGivesObserversIndependentBuffers(t *testing.T) {
	hostile, receivers := newRetainedBroadcastFixture(t, 2)
	hostile.broadcastFrame(func() wire.Frame {
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
	assertIndependentFrames(t, receivers[0].frame, receivers[1].frame)
}

func TestHostileBroadcastFrameReleasesKnownBufferBeforeDelivery(t *testing.T) {
	state := world.New()
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)
	receiver := &nestedFrameReceiver{id: 2}
	state.Spawn(receiver, 100, 0, 0, 0)
	receiver.nested = func() {
		if err := hostile.broadcastFrame(func() wire.Frame {
			return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
		}); err != nil {
			t.Errorf("nested broadcast: %v", err)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- hostile.broadcastFrame(func() wire.Frame {
			return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("broadcastFrame: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast held KnownBuffer while delivering a frame")
	}
}

func TestAITickCompletesWhenBroadcastRecipientRejectsFrame(t *testing.T) {
	state := world.New()
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)
	state.Spawn(&rejectedFrameReceiver{id: 2}, 100, 0, 0, 0)

	ai := task.NewAI(nil)
	ai.Add(&broadcastAIActor{id: 3, hostile: hostile})
	done := make(chan error, 1)
	go func() { done <- ai.Tick() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AI.Tick() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AI.Tick blocked on a rejected broadcast recipient")
	}
}

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

type nestedFrameReceiver struct {
	world.Presence
	id     int32
	once   atomic.Bool
	nested func()
}

type rejectedFrameReceiver struct {
	world.Presence
	id int32
}

func (r *rejectedFrameReceiver) ObjectID() int32 { return r.id }

func (r *rejectedFrameReceiver) BroadcastFrame(frame wire.Frame) bool {
	frame.Release()
	return false
}

type broadcastAIActor struct {
	world.Presence
	id      int32
	hostile *Hostile
}

func (a *broadcastAIActor) ObjectID() int32 { return a.id }
func (*broadcastAIActor) Tick()             {}
func (a *broadcastAIActor) Think() error {
	return a.hostile.broadcastFrame(func() wire.Frame {
		return wire.BorrowedFrame(wire.FrameBytes([]byte{1, 2, 3}))
	})
}

func (r *nestedFrameReceiver) ObjectID() int32 { return r.id }

func (r *nestedFrameReceiver) SendFrame(frame wire.Frame) bool {
	frame.Release()
	if r.once.CompareAndSwap(false, true) {
		r.nested()
	}
	return true
}

func (r *nestedFrameReceiver) BroadcastFrame(frame wire.Frame) bool {
	return r.SendFrame(frame)
}

func (r *retainedFrameReceiver) ObjectID() int32 { return r.id }

func (r *retainedFrameReceiver) SendFrame(frame wire.Frame) bool {
	r.frame = frame
	r.frames++
	return true
}

func (r *retainedFrameReceiver) BroadcastFrame(frame wire.Frame) bool {
	return r.SendFrame(frame)
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
