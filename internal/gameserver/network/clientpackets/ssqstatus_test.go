package clientpackets

import "testing"

func TestDecodeRequestSSQStatus(t *testing.T) {
	got, err := DecodeRequestSSQStatus([]byte{OpcodeRequestSSQStatus, 0x03})
	if err != nil || got.Page != 3 {
		t.Fatalf("decode = (%+v, %v), want page 3", got, err)
	}
	if _, err := DecodeRequestSSQStatus([]byte{OpcodeRequestSSQStatus}); err == nil {
		t.Fatal("decode without a page = nil error")
	}
}
