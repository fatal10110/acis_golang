package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from acquireskill_test.go ----
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

// ---- from actionfailed_test.go ----
func TestFrameActionFailed(t *testing.T) {
	got := framePayload(t, FrameActionFailed())
	want := []byte{OpcodeActionFailed}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameActionFailed() = %x, want %x", got, want)
	}
}

// ---- from magic_skill_test.go ----
func TestFrameMagicSkillUse(t *testing.T) {
	got := framePayload(t, FrameMagicSkillUse(
		SkillCastObject{ObjectID: 100, Location: location.Location{X: 10, Y: 20, Z: 30}},
		SkillCastObject{ObjectID: 200, Location: location.Location{X: 40, Y: 50, Z: 60}},
		3, 1, 500, 1200, false,
	))

	want := []byte{OpcodeMagicSkillUse}
	for _, v := range []uint32{100, 200, 3, 1, 500, 1200, 10, 20, 30, 0, 40, 50, 60} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillUse() = %x, want %x", got, want)
	}
}

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

func TestFrameSetupGauge(t *testing.T) {
	got := framePayload(t, FrameSetupGauge(GaugeBlue, 500, 1200))

	want := []byte{OpcodeSetupGauge}
	for _, v := range []uint32{uint32(GaugeBlue), 500, 1200} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSetupGauge() = %x, want %x", got, want)
	}
}

func TestFrameMagicSkillCanceled(t *testing.T) {
	got := framePayload(t, FrameMagicSkillCanceled(100))
	want := []byte{OpcodeMagicSkillCanceled, 100, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillCanceled() = %x, want %x", got, want)
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

func TestFrameExEnchantSkillInfo(t *testing.T) {
	got := framePayload(t, FrameExEnchantSkillInfo(EnchantSkillInfo{
		ID:     124,
		Level:  101,
		SPCost: 250000,
		XPCost: 123456789,
		Rate:   82,
		Requirements: []EnchantSkillRequirement{
			{Type: 4, ItemID: 6622, Count: 1, Unknown: 0},
		},
	}))

	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExEnchantSkillInfo)
	want = appendD(want, 124)
	want = appendD(want, 101)
	want = appendD(want, 250000)
	want = appendQ(want, 123456789)
	want = appendD(want, 82)
	want = appendD(want, 1)
	want = appendD(want, 4)
	want = appendD(want, 6622)
	want = appendD(want, 1)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExEnchantSkillInfo() = %x, want %x", got, want)
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

func TestFrameShortBuffStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameShortBuffStatusUpdate(1323, 1, 120))

	want := []byte{OpcodeShortBuffStatusUpdate}
	for _, v := range []uint32{1323, 1, 120} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShortBuffStatusUpdate() = %x, want %x", got, want)
	}
}

func appendEffect(out []byte, skillID uint32, level uint16, duration int32) []byte {
	out = binary.LittleEndian.AppendUint32(out, skillID)
	out = binary.LittleEndian.AppendUint16(out, level)
	return binary.LittleEndian.AppendUint32(out, uint32(duration))
}
