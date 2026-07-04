package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeLoginOk(t *testing.T) {
	got := EncodeLoginOk(111, 222)

	var want []byte
	want = append(want, OpcodeLoginOk)
	want = binary.LittleEndian.AppendUint32(want, 111)
	want = binary.LittleEndian.AppendUint32(want, 222)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0x000003ea)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = append(want, make([]byte, 16)...)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeLoginOk = %x, want %x", got, want)
	}
}
