package clientpackets

import "testing"

// TestDecodeRequestQuestAbort pins RequestQuestAbort: opcode 0x64 and one
// little-endian int32 quest id.
func TestDecodeRequestQuestAbort(t *testing.T) {
	got, err := DecodeRequestQuestAbort([]byte{0x64, 0x05, 0x01, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestQuestAbort: %v", err)
	}
	if want := (RequestQuestAbort{QuestID: 261}); got != want {
		t.Errorf("DecodeRequestQuestAbort = %+v, want %+v", got, want)
	}
	if _, err := DecodeRequestQuestAbort([]byte{OpcodeRequestQuestAbort, 0x05, 0x01, 0x00}); err == nil {
		t.Error("DecodeRequestQuestAbort: want error on short payload, got nil")
	}
}
