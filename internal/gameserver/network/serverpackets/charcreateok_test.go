package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameCharCreateOk(t *testing.T) {
	got := framePayload(t, FrameCharCreateOk())

	want := []byte{OpcodeCharCreateOk}
	want = binary.LittleEndian.AppendUint32(want, 1)

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharCreateOk = %x, want %x", got, want)
	}
}
