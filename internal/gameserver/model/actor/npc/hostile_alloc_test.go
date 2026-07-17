//go:build !race

package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestHostileBroadcastMoveUsesReusableKnownSnapshot(t *testing.T) {
	hostile, event, receivers := newBroadcastMoveFixture(t, 50)
	hostile.BroadcastMove(event)

	allocs := testing.AllocsPerRun(100, func() {
		hostile.BroadcastMove(event)
	})
	if allocs != 0 {
		t.Fatalf("BroadcastMove() allocations = %v, want 0 with reusable known-list snapshot", allocs)
	}
	for _, receiver := range receivers {
		if receiver.frames == 0 {
			t.Fatalf("receiver %d got no movement frames", receiver.id)
		}
	}
}

func BenchmarkHostileBroadcastMoveKnownObservers(b *testing.B) {
	hostile, event, _ := newBroadcastMoveFixture(b, 50)
	hostile.BroadcastMove(event)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hostile.BroadcastMove(event)
	}
}

func newBroadcastMoveFixture(tb testing.TB, observers int) (*Hostile, move.Event, []*allocFrameReceiver) {
	tb.Helper()
	state := world.New()
	hostile := newCombatHostile(tb, 1, &Template{ID: 1, Type: "Monster"})
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)

	receivers := make([]*allocFrameReceiver, 0, observers)
	for i := 0; i < observers; i++ {
		receiver := &allocFrameReceiver{id: int32(100 + i)}
		receivers = append(receivers, receiver)
		state.Spawn(receiver, 100+i, 0, 0, 0)
	}

	event := move.Event{
		Origin:      location.Location{X: 0, Y: 0, Z: 0},
		Destination: location.Location{X: 200, Y: 0, Z: 0},
		Speed:       120,
	}
	return hostile, event, receivers
}

type allocFrameReceiver struct {
	world.Presence
	id     int32
	frames int
}

func (r *allocFrameReceiver) ObjectID() int32 { return r.id }

func (r *allocFrameReceiver) SendFrame(frame wire.Frame) bool {
	frame.Release()
	r.frames++
	return true
}
