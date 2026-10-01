package serverpackets

import (
	"bytes"
	"testing"
)

// TestFramePledgeSkillListAdd pins PledgeSkillListAdd: writeC 0xfe,
// writeH 0x3a, writeD skill id, writeD skill level.
func TestFramePledgeSkillListAdd(t *testing.T) {
	got := framePayload(t, FramePledgeSkillListAdd(370, 3))
	want := []byte{0xfe, 0x3a, 0x00}
	want = appendD(want, 370)
	want = appendD(want, 3)
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeSkillListAdd() = %x, want %x", got, want)
	}
}

// TestFramePledgeSkillListEmpty pins an empty clan skill list: writeC 0xfe,
// writeH 0x39, writeD 0.
func TestFramePledgeSkillListEmpty(t *testing.T) {
	got := framePayload(t, FramePledgeSkillList(nil))
	want := appendD([]byte{0xfe, 0x39, 0x00}, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeSkillList(nil) = %x, want %x", got, want)
	}
}
