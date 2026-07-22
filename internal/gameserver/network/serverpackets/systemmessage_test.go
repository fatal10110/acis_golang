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

func TestFrameSystemMessageSkillName(t *testing.T) {
	got := framePayload(t, FrameSystemMessageSkillName(SystemMessageNightSkillEffectApplies, 294, 1))
	want := []byte{
		OpcodeSystemMessage,
		0x6b, 0x04, 0x00, 0x00, // 1131
		0x01, 0x00, 0x00, 0x00, // one param
		0x04, 0x00, 0x00, 0x00, // skill-name param
		0x26, 0x01, 0x00, 0x00, // skill 294
		0x01, 0x00, 0x00, 0x00, // level 1
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageSkillName() = %x, want %x", got, want)
	}
}
