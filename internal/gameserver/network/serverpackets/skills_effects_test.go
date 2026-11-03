package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ---- from acquireskill_test.go ----
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

// ---- from actionfailed_test.go ----
// ---- from magic_skill_test.go ----
func TestFrameMagicSkillLaunched(t *testing.T) {
	got := framePayload(t, FrameMagicSkillLaunched(100, 3, 1, []int32{200, 300}))

	want := []byte{OpcodeMagicSkillLaunched}
	for _, v := range []uint32{100, 3, 1, 2, 200, 300} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillLaunched() = %x, want %x", got, want)
	}
}

func TestFrameMagicSkillLaunchedNoTargets(t *testing.T) {
	got := framePayload(t, FrameMagicSkillLaunched(100, 3, 1, nil))

	want := []byte{OpcodeMagicSkillLaunched}
	for _, v := range []uint32{100, 3, 1, 0, 0} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillLaunched(nil) = %x, want %x", got, want)
	}
}

// ---- from skill_enchant_test.go ----
func TestFrameExEnchantSkillList(t *testing.T) {
	got := framePayload(t, FrameExEnchantSkillList([]EnchantSkillEntry{
		{ID: 124, Level: 101, SPCost: 250000, XPCost: 123456789},
		{ID: 125, Level: 102, SPCost: 350000, XPCost: 987654321},
	}))

	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExEnchantSkillList)
	want = appendD(want, 2)
	want = appendD(want, 124)
	want = appendD(want, 101)
	want = appendD(want, 250000)
	want = appendQ(want, 123456789)
	want = appendD(want, 125)
	want = appendD(want, 102)
	want = appendD(want, 350000)
	want = appendQ(want, 987654321)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExEnchantSkillList() = %x, want %x", got, want)
	}
}

// ---- from skilllist_test.go ----
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

// ---- from status_effects_test.go ----
func TestFrameAbnormalStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameAbnormalStatusUpdate([]AbnormalStatusEffect{
		{SkillID: 1040, Level: 3, DurationMillis: 15_000},
		{SkillID: 1068, Level: 2, DurationMillis: -1},
		{SkillID: 1002, Level: 1, DurationMillis: 30_000, Toggle: true},
		{SkillID: 1001, Level: 4, DurationMillis: 30_000, Toggle: true},
	}))

	want := []byte{OpcodeAbnormalStatusUpdate}
	want = binary.LittleEndian.AppendUint16(want, 4)
	want = appendEffect(want, 1040, 3, 15)
	want = appendEffect(want, 1068, 2, -1)
	want = appendEffect(want, 1001, 4, -1)
	want = appendEffect(want, 1002, 1, -1)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAbnormalStatusUpdate() = %x, want %x", got, want)
	}
}

func appendEffect(out []byte, skillID uint32, level uint16, duration int32) []byte {
	out = binary.LittleEndian.AppendUint32(out, skillID)
	out = binary.LittleEndian.AppendUint16(out, level)
	return binary.LittleEndian.AppendUint32(out, uint32(duration))
}
