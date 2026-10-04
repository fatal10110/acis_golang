package clientpackets

import "testing"

func TestDecodeObserverReturn(t *testing.T) {
	if _, err := DecodeObserverReturn([]byte{OpcodeObserverReturn}); err != nil {
		t.Fatalf("DecodeObserverReturn: %v", err)
	}
	if _, err := DecodeObserverReturn(nil); err == nil {
		t.Fatal("DecodeObserverReturn: want error on short payload")
	}
}
