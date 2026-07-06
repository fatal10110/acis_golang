package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeCharCreateOk(t *testing.T) {
	got := EncodeCharCreateOk()

	want := []byte{OpcodeCharCreateOk}
	want = binary.LittleEndian.AppendUint32(want, 1)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeCharCreateOk = %x, want %x", got, want)
	}
}
