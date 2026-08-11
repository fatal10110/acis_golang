package serverpackets

import (
	"encoding/binary"
	"testing"
)

func TestFrameRideMountWyvern(t *testing.T) {
	want := []byte{OpcodeRide}
	want = binary.LittleEndian.AppendUint32(want, 7)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 1012621)
	if got := framePayload(t, FrameRide(7, 12621)); string(got) != string(want) {
		t.Fatalf("Ride = %x, want %x", got, want)
	}
}
