package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

// ---- from attack_test.go ----
func TestFrameAttack(t *testing.T) {
	got := framePayload(t, FrameAttack(AttackSnapshot{
		AttackerID: 268476516,
		X:          -71440,
		Y:          258000,
		Z:          -3104,
		Hits: []AttackHit{
			{TargetID: 268480061, Damage: 37, Flags: AttackHitSoulshot | AttackHitCritical | AttackHitShield},
			{TargetID: 7, Damage: 0, Flags: AttackHitMiss},
		},
	}))
	want := []byte{
		0x05,
		0x64, 0xa0, 0x00, 0x10,
		0x3d, 0xae, 0x00, 0x10,
		0x25, 0x00, 0x00, 0x00,
		0x70,
		0xf0, 0xe8, 0xfe, 0xff,
		0xd0, 0xef, 0x03, 0x00,
		0xe0, 0xf3, 0xff, 0xff,
		0x01, 0x00,
		0x07, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x80,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAttack() = %x, want %x", got, want)
	}
}

// ---- from autoattack_test.go ----
// ---- from deleteobject_test.go ----
// ---- from flytolocation_test.go ----
// ---- from move_test.go ----
func TestFrameMoveLocationEvent(t *testing.T) {
	ev := event.Move{
		Origin:      location.Location{X: 10, Y: 20, Z: 30},
		Destination: location.Location{X: 100, Y: 200, Z: 300},
	}
	got := framePayload(t, FrameMove(101, ev))
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
	ev := event.Move{
		Origin:       location.Location{X: 10, Y: 20, Z: 30},
		FollowTarget: 202,
		FollowOffset: 40,
	}
	got := framePayload(t, FrameMove(101, ev))
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

// ---- from movement_control_test.go ----
// ---- from movetolocation_test.go ----
// ---- from movetopawn_test.go ----
// ---- from stance_social_test.go ----
// ---- from targeting_test.go ----
// ---- from teleporttolocation_test.go ----
// ---- from worldobjects_test.go ----
type doorShape struct{}

func (doorShape) GeoX() int { return 0 }

func (doorShape) GeoY() int { return 0 }

func (doorShape) GeoZ() int { return 0 }

func (doorShape) Height() int { return 32 }

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
