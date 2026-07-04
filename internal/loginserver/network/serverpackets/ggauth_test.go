package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeGGAuth(t *testing.T) {
	got := EncodeGGAuth(GGAuthSkipRequest)

	var want []byte
	want = append(want, OpcodeGGAuth)
	want = binary.LittleEndian.AppendUint32(want, uint32(GGAuthSkipRequest))
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeGGAuth = %x, want %x", got, want)
	}
}
