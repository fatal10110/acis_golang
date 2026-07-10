package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameCharDeleteOk(t *testing.T) {
	got := framePayload(t, FrameCharDeleteOk())
	want := []byte{OpcodeCharDeleteOk}
	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharDeleteOk = %x, want %x", got, want)
	}
}
