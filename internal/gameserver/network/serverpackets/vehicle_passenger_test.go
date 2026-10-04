package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The expected bytes below are written out by hand from the reference
// writeImpl layouts (network/serverpackets/GetOnVehicle.java,
// GetOffVehicle.java, MoveToLocationInVehicle.java, StopMoveInVehicle.java
// and ValidateLocationInVehicle.java), not produced by the encoders under
// test.

func assertFrameBytes(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s = % x, want % x", name, got, want)
	}
}

// GetOnVehicle: 0x5c, d player id, d boat id, d x, d y, d z.
func TestGetOnVehicleBytes(t *testing.T) {
	got := framePayload(t, FrameGetOnVehicle(0x0a0b0c0d, 7, location.Location{X: 230, Y: -260, Z: -40}))
	assertFrameBytes(t, "GetOnVehicle", got, []byte{
		0x5c,
		0x0d, 0x0c, 0x0b, 0x0a,
		0x07, 0x00, 0x00, 0x00,
		0xe6, 0x00, 0x00, 0x00, // 230
		0xfc, 0xfe, 0xff, 0xff, // -260
		0xd8, 0xff, 0xff, 0xff, // -40
	})
}

// GetOffVehicle: 0x5d, d player id, d boat id, d x, d y, d z.
func TestGetOffVehicleBytes(t *testing.T) {
	got := framePayload(t, FrameGetOffVehicle(0x0a0b0c0d, 7, location.Location{X: -96622, Y: 261660, Z: -3610}))
	assertFrameBytes(t, "GetOffVehicle", got, []byte{
		0x5d,
		0x0d, 0x0c, 0x0b, 0x0a,
		0x07, 0x00, 0x00, 0x00,
		0x92, 0x86, 0xfe, 0xff, // -96622
		0x1c, 0xfe, 0x03, 0x00, // 261660
		0xe6, 0xf1, 0xff, 0xff, // -3610
	})
}

// MoveToLocationInVehicle: 0x71, d player id, d boat id, the target x, y,
// z, then the origin x, y, z.
func TestMoveToLocationInVehicleBytes(t *testing.T) {
	got := framePayload(t, FrameMoveToLocationInVehicle(0x0a0b0c0d, 7,
		location.Location{X: 230, Y: -260, Z: -48}, location.Location{X: 10, Y: -100, Z: -40}))
	assertFrameBytes(t, "MoveToLocationInVehicle", got, []byte{
		0x71,
		0x0d, 0x0c, 0x0b, 0x0a,
		0x07, 0x00, 0x00, 0x00,
		0xe6, 0x00, 0x00, 0x00, // 230
		0xfc, 0xfe, 0xff, 0xff, // -260
		0xd0, 0xff, 0xff, 0xff, // -48
		0x0a, 0x00, 0x00, 0x00, // 10
		0x9c, 0xff, 0xff, 0xff, // -100
		0xd8, 0xff, 0xff, 0xff, // -40
	})
}

// StopMoveInVehicle: 0x72, d player id, d boat id, d x, d y, d z, d heading.
func TestStopMoveInVehicleBytes(t *testing.T) {
	got := framePayload(t, FrameStopMoveInVehicle(0x0a0b0c0d, 7, location.Location{X: 10, Y: -100, Z: -40}, 32768))
	assertFrameBytes(t, "StopMoveInVehicle", got, []byte{
		0x72,
		0x0d, 0x0c, 0x0b, 0x0a,
		0x07, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00, // 10
		0x9c, 0xff, 0xff, 0xff, // -100
		0xd8, 0xff, 0xff, 0xff, // -40
		0x00, 0x80, 0x00, 0x00, // 32768
	})
}

// ValidateLocationInVehicle: 0x73, d player id, d boat id, d x, d y, d z,
// d heading.
func TestValidateLocationInVehicleBytes(t *testing.T) {
	got := framePayload(t, FrameValidateLocationInVehicle(0x0a0b0c0d, 7, location.Location{X: 230, Y: -260, Z: -40}, 61440))
	assertFrameBytes(t, "ValidateLocationInVehicle", got, []byte{
		0x73,
		0x0d, 0x0c, 0x0b, 0x0a,
		0x07, 0x00, 0x00, 0x00,
		0xe6, 0x00, 0x00, 0x00, // 230
		0xfc, 0xfe, 0xff, 0xff, // -260
		0xd8, 0xff, 0xff, 0xff, // -40
		0x00, 0xf0, 0x00, 0x00, // 61440
	})
}
