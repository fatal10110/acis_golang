package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

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
