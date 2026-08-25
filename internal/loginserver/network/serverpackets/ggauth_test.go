package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeGGAuth(t *testing.T) {
	const sessionID int32 = 0x11223344
	got := EncodeGGAuth(sessionID)
	want := make([]byte, 0, 21)
	want = append(want, OpcodeGGAuth)
	want = binary.LittleEndian.AppendUint32(want, uint32(sessionID))
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeGGAuth = %x, want %x", got, want)
	}
}
