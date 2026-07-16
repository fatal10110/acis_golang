package clientpackets

import "testing"

func TestDecodeAppearing(t *testing.T) {
	if _, err := DecodeAppearing([]byte{OpcodeAppearing}); err != nil {
		t.Fatalf("DecodeAppearing: %v", err)
	}
}

func TestDecodeAppearingShort(t *testing.T) {
	if _, err := DecodeAppearing(nil); err == nil {
		t.Fatal("DecodeAppearing: want error on short payload")
	}
}
