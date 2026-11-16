package clientpackets

import (
	"encoding/binary"
	"testing"
)

// TestDecodeRequestCharacterCreateTruncatesSex pins the sex field's read: only
// the low byte of the 32-bit value counts, sign included
// (RequestCharacterCreate.java readImpl, `(byte) readD()`).
func TestDecodeRequestCharacterCreateTruncatesSex(t *testing.T) {
	for _, tc := range []struct {
		wire uint32
		want int8
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{0x100, 0},
		{0x101, 1},
		{0x7f, 127},
		{0xff, -1},
		{0xffffffff, -1},
		{0x180, -128},
	} {
		var payload []byte
		payload = append(payload, OpcodeRequestCharacterCreate)
		payload = append(payload, encodeUTF16Z("Newbie")...)
		payload = binary.LittleEndian.AppendUint32(payload, 0) // race
		payload = binary.LittleEndian.AppendUint32(payload, tc.wire)
		payload = binary.LittleEndian.AppendUint32(payload, 0) // classId
		for range 9 {
			payload = binary.LittleEndian.AppendUint32(payload, 0)
		}
		got, err := DecodeRequestCharacterCreate(payload)
		if err != nil {
			t.Fatalf("sex %#x: %v", tc.wire, err)
		}
		if got.Sex != tc.want {
			t.Errorf("sex %#x decoded as %d, want %d", tc.wire, got.Sex, tc.want)
		}
	}
}
