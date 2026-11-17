package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestFrameSystemMessageParamsSkillName: a skill-name parameter through the
// generic writer carries the skill id then its level, byte for byte what the
// dedicated skill-name writer sends.
func TestFrameSystemMessageParamsSkillName(t *testing.T) {
	got := FrameSystemMessageParams(1260, SkillNameParam(1204, 2))
	defer got.Release()
	want := FrameSystemMessageSkillName(1260, 1204, 2)
	defer want.Release()
	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Fatalf("FrameSystemMessageParams(skill name) = % x, want % x", got.Bytes(), want.Bytes())
	}

	got2 := FrameSystemMessageParams(1260, TextParam("x"), SkillNameParam(1204, 2))
	defer got2.Release()
	want2 := FrameSystemMessageStringSkillName(1260, "x", 1204, 2)
	defer want2.Release()
	if !bytes.Equal(got2.Bytes(), want2.Bytes()) {
		t.Fatalf("FrameSystemMessageParams(text, skill name) = % x, want % x", got2.Bytes(), want2.Bytes())
	}
}

// TestFrameSystemMessageParamsNpcName: an NPC-name parameter (type 2) is one
// int32 after its type, like a number.
func TestFrameSystemMessageParamsNpcName(t *testing.T) {
	const npcNameType = 2
	frame := FrameSystemMessageParams(7, SystemMessageParam{Type: npcNameType, Value: 1030001})
	defer frame.Release()
	payload := frame.Bytes()[2:]
	want := []byte{OpcodeSystemMessage}
	for _, v := range []int32{7, 1, npcNameType, 1030001} {
		want = binary.LittleEndian.AppendUint32(want, uint32(v))
	}
	if !bytes.Equal(payload, want) {
		t.Fatalf("FrameSystemMessageParams(npc name) payload = % x, want % x", payload, want)
	}
}
