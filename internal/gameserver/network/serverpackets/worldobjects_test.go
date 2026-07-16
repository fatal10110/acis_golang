package serverpackets

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

type doorShape struct{}

func (doorShape) GeoX() int               { return 0 }
func (doorShape) GeoY() int               { return 0 }
func (doorShape) GeoZ() int               { return 0 }
func (doorShape) Height() int             { return 32 }
func (doorShape) GeoData() [][]block.NSWE { return [][]block.NSWE{{block.NoDirections}} }

func TestFrameDoorInfo(t *testing.T) {
	gate := testDoor(t)

	got := framePayload(t, FrameDoorInfo(gate, true))

	want := []byte{OpcodeDoorInfo}
	want = binary.LittleEndian.AppendUint32(want, uint32(1000))
	want = binary.LittleEndian.AppendUint32(want, uint32(19210001))
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 253200)
	want = binary.LittleEndian.AppendUint32(want, 253200)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	if string(got) != string(want) {
		t.Fatalf("FrameDoorInfo() = % x, want % x", got, want)
	}
}

func TestFrameDoorStatusUpdate(t *testing.T) {
	gate := testDoor(t)

	got := framePayload(t, FrameDoorStatusUpdate(gate, false))

	want := []byte{OpcodeDoorStatusUpdate}
	want = binary.LittleEndian.AppendUint32(want, uint32(1000))
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(19210001))
	want = binary.LittleEndian.AppendUint32(want, 253200)
	want = binary.LittleEndian.AppendUint32(want, 253200)
	if string(got) != string(want) {
		t.Fatalf("FrameDoorStatusUpdate() = % x, want % x", got, want)
	}
}

func TestFrameStaticObjectInfo(t *testing.T) {
	sign, err := staticobject.NewObject(1001, &staticobject.Template{ID: 41001})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	got := framePayload(t, FrameStaticObjectInfo(sign))

	want := []byte{OpcodeStaticObjectInfo}
	want = binary.LittleEndian.AppendUint32(want, 41001)
	want = binary.LittleEndian.AppendUint32(want, 1001)
	if string(got) != string(want) {
		t.Fatalf("FrameStaticObjectInfo() = % x, want % x", got, want)
	}
}

func TestFrameChairSit(t *testing.T) {
	got := framePayload(t, FrameChairSit(0x1000a064, 1234))

	want := []byte{OpcodeChairSit}
	want = binary.LittleEndian.AppendUint32(want, 0x1000a064)
	want = binary.LittleEndian.AppendUint32(want, 1234)
	if string(got) != string(want) {
		t.Fatalf("FrameChairSit() = % x, want % x", got, want)
	}
}

func testDoor(t *testing.T) *door.Object {
	t.Helper()
	gate, err := door.NewObject(1000, &door.Template{
		ID:       19210001,
		Name:     "gludio_castle_outter_001",
		Kind:     door.KindDoor,
		Level:    1,
		Position: location.Location{X: -18408, Y: 113064, Z: -2768},
		Coordinates: []location.Point{
			{X: -18481, Y: 113059},
			{X: -18351, Y: 113059},
			{X: -18351, Y: 113071},
			{X: -18481, Y: 113071},
		},
		HP: 253200, PDef: 644, MDef: 518, Height: 320,
	}, doorShape{})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	return gate
}
