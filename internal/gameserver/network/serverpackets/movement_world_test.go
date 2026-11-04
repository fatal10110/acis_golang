package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// FrameMove picks the packet a move event goes out as; the packets' bytes
// are pinned by the MoveToLocation and MoveToPawn rows of
// testdata/packets.golden.
func TestFrameMoveLocationEvent(t *testing.T) {
	origin, dest := location.Location{X: 10, Y: 20, Z: 30}, location.Location{X: 100, Y: 200, Z: 300}
	got := framePayload(t, FrameMove(101, event.Move{Origin: origin, Destination: dest}))
	if want := framePayload(t, FrameMoveToLocation(101, dest, origin)); !bytes.Equal(got, want) {
		t.Fatalf("FrameMove() = %x, want MoveToLocation %x", got, want)
	}
}

func TestFrameMoveFollowEvent(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	got := framePayload(t, FrameMove(101, event.Move{Origin: origin, FollowTarget: 202, FollowOffset: 40}))
	if want := framePayload(t, FrameMoveToPawn(101, 202, 40, origin)); !bytes.Equal(got, want) {
		t.Fatalf("FrameMove() = %x, want MoveToPawn %x", got, want)
	}
}
