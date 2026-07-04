package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodePlayOk(t *testing.T) {
	got := EncodePlayOk(333, 444)

	var want []byte
	want = append(want, OpcodePlayOk)
	want = binary.LittleEndian.AppendUint32(want, 333)
	want = binary.LittleEndian.AppendUint32(want, 444)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodePlayOk = %x, want %x", got, want)
	}
}
