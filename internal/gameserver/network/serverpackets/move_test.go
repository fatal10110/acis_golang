package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameMoveLocationEvent(t *testing.T) {
	event := move.Event{
		Origin:      location.Location{X: 10, Y: 20, Z: 30},
		Destination: location.Location{X: 100, Y: 200, Z: 300},
	}
	got := framePayload(t, FrameMove(101, event))
	want := []byte{
		0x01,
		0x65, 0x00, 0x00, 0x00,
		0x64, 0x00, 0x00, 0x00,
		0xc8, 0x00, 0x00, 0x00,
		0x2c, 0x01, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x14, 0x00, 0x00, 0x00,
		0x1e, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMove() = %x, want MoveToLocation %x", got, want)
	}
}

func TestFrameMoveFollowEvent(t *testing.T) {
	event := move.Event{
		Origin:       location.Location{X: 10, Y: 20, Z: 30},
		FollowTarget: 202,
		FollowOffset: 40,
	}
	got := framePayload(t, FrameMove(101, event))
	want := []byte{
		0x60,
		0x65, 0x00, 0x00, 0x00,
		0xca, 0x00, 0x00, 0x00,
		0x28, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x14, 0x00, 0x00, 0x00,
		0x1e, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMove() = %x, want MoveToPawn %x", got, want)
	}
}
