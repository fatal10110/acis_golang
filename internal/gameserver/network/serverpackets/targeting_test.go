package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameMyTargetSelected(t *testing.T) {
	got := framePayload(t, FrameMyTargetSelected(12345, 0x0010))
	want := []byte{
		OpcodeMyTargetSelected,
		0x39, 0x30, 0x00, 0x00,
		0x10, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMyTargetSelected() = %x, want %x", got, want)
	}
}

func TestFrameTargetSelected(t *testing.T) {
	got := framePayload(t, FrameTargetSelected(100, 200, location.Location{X: 46160, Y: 41237, Z: -3534}))
	want := []byte{
		OpcodeTargetSelected,
		0x64, 0x00, 0x00, 0x00,
		0xc8, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTargetSelected() = %x, want %x", got, want)
	}
}

func TestFrameTargetUnselected(t *testing.T) {
	got := framePayload(t, FrameTargetUnselected(100, location.Location{X: 46160, Y: 41237, Z: -3534}))
	want := []byte{
		OpcodeTargetUnselected,
		0x64, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTargetUnselected() = %x, want %x", got, want)
	}
}

func TestFrameStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameStatusUpdate(12345, []StatusAttribute{
		{Type: StatusCurrentHP, Value: 75},
		{Type: StatusMaxHP, Value: 100},
	}))
	want := []byte{
		OpcodeStatusUpdate,
		0x39, 0x30, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x09, 0x00, 0x00, 0x00,
		0x4b, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x64, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStatusUpdate() = %x, want %x", got, want)
	}
}
