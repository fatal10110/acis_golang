package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameSkillList_Empty(t *testing.T) {
	got := framePayload(t, FrameSkillList(nil))
	want := []byte{OpcodeSkillList, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("FrameSkillList(nil) = %x, want %x", got, want)
	}
}

func TestFrameSkillList_Entries(t *testing.T) {
	skills := []SkillListEntry{
		{ID: 1001, Level: 3, Passive: false, Disabled: false},
		{ID: 1002, Level: 1, Passive: true, Disabled: true},
	}
	got := framePayload(t, FrameSkillList(skills))

	want := []byte{OpcodeSkillList}
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 0) // not passive
	want = binary.LittleEndian.AppendUint32(want, 3) // level
	want = binary.LittleEndian.AppendUint32(want, 1001)
	want = append(want, 0)                           // not disabled
	want = binary.LittleEndian.AppendUint32(want, 1) // passive
	want = binary.LittleEndian.AppendUint32(want, 1) // level
	want = binary.LittleEndian.AppendUint32(want, 1002)
	want = append(want, 1) // disabled

	if !bytes.Equal(got, want) {
		t.Errorf("FrameSkillList() = %x, want %x", got, want)
	}
}
