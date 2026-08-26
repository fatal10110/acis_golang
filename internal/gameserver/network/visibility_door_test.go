package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestDiscoverDoorSendsInfoThenStatus(t *testing.T) {
	gate, err := door.NewObject(1000, &door.Template{ID: 19210001, HP: 253200}, visibilityDoorShape{})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	var frames [][]byte
	p := &livePlayer{visibilitySend: func(frame wire.Frame) bool {
		frames = append(frames, append([]byte(nil), frame.Bytes()...))
		frame.Release()
		return true
	}}
	p.Discover(gate)

	if len(frames) != 2 {
		t.Fatalf("door visibility frames = %d, want 2", len(frames))
	}
	if got := frames[0][wire.FrameHeaderSize]; got != serverpackets.OpcodeDoorInfo {
		t.Fatalf("first door visibility opcode = %#x, want DoorInfo %#x", got, serverpackets.OpcodeDoorInfo)
	}
	if got := frames[1][wire.FrameHeaderSize]; got != serverpackets.OpcodeDoorStatusUpdate {
		t.Fatalf("second door visibility opcode = %#x, want DoorStatusUpdate %#x", got, serverpackets.OpcodeDoorStatusUpdate)
	}
}

type visibilityDoorShape struct{}

func (visibilityDoorShape) GeoX() int               { return 1 }
func (visibilityDoorShape) GeoY() int               { return 1 }
func (visibilityDoorShape) GeoZ() int               { return 1 }
func (visibilityDoorShape) Height() int             { return 1 }
func (visibilityDoorShape) GeoData() [][]block.NSWE { return [][]block.NSWE{{0}} }
