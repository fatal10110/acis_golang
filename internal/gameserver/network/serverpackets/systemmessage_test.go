package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameSystemMessage(t *testing.T) {
	got := framePayload(t, FrameSystemMessage(SystemMessagePetRefusingOrder))
	want := []byte{
		OpcodeSystemMessage,
		0x48, 0x07, 0x00, 0x00, // 1864
		0x00, 0x00, 0x00, 0x00, // no params
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessage() = %x, want %x", got, want)
	}
}
