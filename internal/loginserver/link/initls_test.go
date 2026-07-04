package link

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeInitLS(t *testing.T) {
	pubKey := bytes.Repeat([]byte{0xaa}, 128)

	got := EncodeInitLS(pubKey)

	var want []byte
	want = append(want, OpcodeInitLS)
	want = binary.LittleEndian.AppendUint32(want, linkProtocolRevision)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(pubKey)))
	want = append(want, pubKey...)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeInitLS() = %x, want %x", got, want)
	}
}
