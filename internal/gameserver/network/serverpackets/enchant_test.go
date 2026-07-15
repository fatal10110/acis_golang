package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameEnchantResult(t *testing.T) {
	got := framePayload(t, FrameEnchantResult(EnchantResultCancelled))
	want := []byte{OpcodeEnchantResult, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameEnchantResult() = %x, want %x", got, want)
	}
}

func TestFrameChooseInventoryItem(t *testing.T) {
	got := framePayload(t, FrameChooseInventoryItem(955))
	want := []byte{OpcodeChooseInventoryItem, 0xbb, 0x03, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChooseInventoryItem() = %x, want %x", got, want)
	}
}
