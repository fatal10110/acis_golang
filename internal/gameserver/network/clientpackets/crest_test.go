package clientpackets

import "testing"

func TestDecodeRequestPledgeCrest(t *testing.T) {
	payload := []byte{
		OpcodeRequestPledgeCrest,
		0x65, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestPledgeCrest(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPledgeCrest: %v", err)
	}
	if got.CrestID != 101 {
		t.Fatalf("CrestID = %d, want 101", got.CrestID)
	}
}

func TestDecodeRequestAllyCrest(t *testing.T) {
	payload := []byte{
		OpcodeRequestAllyCrest,
		0x67, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAllyCrest(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAllyCrest: %v", err)
	}
	if got.CrestID != 103 {
		t.Fatalf("CrestID = %d, want 103", got.CrestID)
	}
}

func TestDecodeRequestPledgeCrestShort(t *testing.T) {
	if _, err := DecodeRequestPledgeCrest([]byte{OpcodeRequestPledgeCrest, 1}); err == nil {
		t.Fatal("DecodeRequestPledgeCrest: want error on short payload")
	}
}

func TestDecodeRequestAllyCrestShort(t *testing.T) {
	if _, err := DecodeRequestAllyCrest([]byte{OpcodeRequestAllyCrest, 1}); err == nil {
		t.Fatal("DecodeRequestAllyCrest: want error on short payload")
	}
}
