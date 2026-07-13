package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameActionFailed(t *testing.T) {
	got := framePayload(t, FrameActionFailed())
	want := []byte{OpcodeActionFailed}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameActionFailed() = %x, want %x", got, want)
	}
}
