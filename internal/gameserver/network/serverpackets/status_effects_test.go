package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

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
