package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The expected bytes below are written out by hand from the reference
// writeImpl layouts (network/serverpackets/VehicleInfo.java,
// VehicleDeparture.java, OnVehicleCheckLocation.java, VehicleStarted.java
// and CreatureSay.java's system-message branch reached through BoatSay.java),
// not produced by the encoders under test.

// VehicleInfo: 0x59, d object id, d x, d y, d z, d heading.
func TestVehicleInfoBytes(t *testing.T) {
	got := framePayload(t, FrameVehicleInfo(0x01020304, location.Location{X: -96622, Y: 261660, Z: -3610}, 32768))
	want := []byte{
		0x59,
		0x04, 0x03, 0x02, 0x01,
		0x92, 0x86, 0xfe, 0xff, // -96622
		0x1c, 0xfe, 0x03, 0x00, // 261660
		0xe6, 0xf1, 0xff, 0xff, // -3610
		0x00, 0x80, 0x00, 0x00, // 32768
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("VehicleInfo = % x, want % x", got, want)
	}
}

// OnVehicleCheckLocation: 0x5b, d object id, d x, d y, d z, d heading.
func TestOnVehicleCheckLocationBytes(t *testing.T) {
	got := framePayload(t, FrameOnVehicleCheckLocation(7, location.Location{X: 48950, Y: 190613, Z: -3610}, 60800))
	want := []byte{
		0x5b,
		0x07, 0x00, 0x00, 0x00,
		0x36, 0xbf, 0x00, 0x00, // 48950
		0x95, 0xe8, 0x02, 0x00, // 190613
		0xe6, 0xf1, 0xff, 0xff, // -3610
		0x80, 0xed, 0x00, 0x00, // 60800
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("OnVehicleCheckLocation = % x, want % x", got, want)
	}
}

// VehicleDeparture: 0x5a, d object id, d move speed, d rotation speed, then
// the destination x, y, z.
func TestVehicleDepartureBytes(t *testing.T) {
	got := framePayload(t, FrameVehicleDeparture(7, 150, 800, location.Location{X: 51914, Y: 189023, Z: -3610}))
	want := []byte{
		0x5a,
		0x07, 0x00, 0x00, 0x00,
		0x96, 0x00, 0x00, 0x00, // 150
		0x20, 0x03, 0x00, 0x00, // 800
		0xca, 0xca, 0x00, 0x00, // 51914
		0x5f, 0xe2, 0x02, 0x00, // 189023
		0xe6, 0xf1, 0xff, 0xff, // -3610
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("VehicleDeparture = % x, want % x", got, want)
	}
}

// VehicleStarted: 0xba, d object id, d state (1 setting off, 0 stopped).
func TestVehicleStartedBytes(t *testing.T) {
	for _, tc := range []struct {
		moving bool
		state  byte
	}{{true, 1}, {false, 0}} {
		got := framePayload(t, FrameVehicleStarted(0x0a0b0c0d, tc.moving))
		want := []byte{0xba, 0x0d, 0x0c, 0x0b, 0x0a, tc.state, 0x00, 0x00, 0x00}
		if !bytes.Equal(got, want) {
			t.Fatalf("VehicleStarted(moving=%v) = % x, want % x", tc.moving, got, want)
		}
	}
}

// BoatSay is CreatureSay's system-message form: 0x4a, d object id 0, d chat
// channel BOAT (11), d sysstring 801, d system message id.
func TestBoatSayBytes(t *testing.T) {
	got := framePayload(t, FrameBoatSay(1223))
	want := []byte{
		0x4a,
		0x00, 0x00, 0x00, 0x00,
		0x0b, 0x00, 0x00, 0x00,
		0x21, 0x03, 0x00, 0x00, // 801
		0xc7, 0x04, 0x00, 0x00, // 1223
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("BoatSay = % x, want % x", got, want)
	}
}
