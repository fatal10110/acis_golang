package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeInit(t *testing.T) {
	modulus := bytes.Repeat([]byte{0xaa}, 128)
	blowfishKey := bytes.Repeat([]byte{0xbb}, 16)

	got := EncodeInit(0x11223344, modulus, blowfishKey)

	var want []byte
	want = append(want, OpcodeInit)
	want = binary.LittleEndian.AppendUint32(want, 0x11223344)
	want = binary.LittleEndian.AppendUint32(want, protocolVersion)
	want = append(want, modulus...)
	want = append(want, make([]byte, 16)...)
	want = append(want, blowfishKey...)
	want = append(want, 0x00)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeInit = %x, want %x", got, want)
	}
}
