package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameAcquireSkillInfo(t *testing.T) {
	got := framePayload(t, FrameAcquireSkillInfo(3, 1, 50, 0, []SkillRequirement{
		{Type: 99, ItemID: 57, Count: 1, Unknown: 50},
	}))

	want := []byte{OpcodeAcquireSkillInfo}
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 50)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 99)
	want = binary.LittleEndian.AppendUint32(want, 57)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 50)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAcquireSkillInfo() = %x, want %x", got, want)
	}
}

func TestFrameAcquireSkillList(t *testing.T) {
	got := framePayload(t, FrameAcquireSkillList(AcquireSkillTypeUsual, []AcquireSkillListEntry{
		{ID: 3, Level: 1, Cost: 50},
		{ID: 4, Level: 2, Cost: 100},
	}))

	want := []byte{OpcodeAcquireSkillList}
	want = binary.LittleEndian.AppendUint32(want, uint32(AcquireSkillTypeUsual))
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 50)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 4)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAcquireSkillList() = %x, want %x", got, want)
	}
}

func TestFrameAcquireSkillDone(t *testing.T) {
	got := framePayload(t, FrameAcquireSkillDone())
	want := []byte{OpcodeAcquireSkillDone}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAcquireSkillDone() = %x, want %x", got, want)
	}
}
