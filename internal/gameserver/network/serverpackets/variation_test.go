package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func appendQ(b []byte, v int64) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(v))
}

func TestFrameExShowVariationWindows(t *testing.T) {
	got := framePayload(t, FrameExShowVariationMakeWindow())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationMakeWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationMakeWindow() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameExShowVariationCancelWindow())
	want = []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationCancelWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationCancelWindow() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationItem(1000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationItem)
	want = appendD(want, 1000)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationItem() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationRefiner(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationRefiner(2000, 8723, 2130, 20))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationRefiner)
	want = appendD(want, 2000)
	want = appendD(want, 8723)
	want = appendD(want, 2130)
	want = appendD(want, 20)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationRefiner() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationGemstone(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationGemstone(3000, 36))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationGemstone)
	want = appendD(want, 3000)
	want = appendD(want, 1)
	want = appendD(want, 36)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationGemstone() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmCancelItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmCancelItem(1000, 7575, 0x12345678, 390000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmCancelItem)
	want = appendD(want, 1000)
	want = appendD(want, 7575)
	want = appendD(want, 0x5678)
	want = appendD(want, 0x1234)
	want = appendQ(want, 390000)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmCancelItem() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResult(t *testing.T) {
	got := framePayload(t, FrameExVariationResult(0x1111, 0x2222, 1))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0x1111)
	want = appendD(want, 0x2222)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResult() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResultFailed(t *testing.T) {
	got := framePayload(t, FrameExVariationResultFailed())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResultFailed() = %x, want %x", got, want)
	}
}

func TestFrameExVariationCancelResult(t *testing.T) {
	got := framePayload(t, FrameExVariationCancelResult(1))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationCancelResult)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationCancelResult() = %x, want %x", got, want)
	}
}
