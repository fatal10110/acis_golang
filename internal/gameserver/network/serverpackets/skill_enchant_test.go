package serverpackets

import (
	"bytes"
	"testing"
)

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
