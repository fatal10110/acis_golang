package clientpackets

import "testing"

func TestDecodeDlgAnswer(t *testing.T) {
	payload := []byte{
		OpcodeDlgAnswer,
		0x32, 0x07, 0x00, 0x00, // messageId 1842
		0x01, 0x00, 0x00, 0x00, // answer accept
		0x39, 0x30, 0x00, 0x00, // requesterId 12345
	}
	got, err := DecodeDlgAnswer(payload)
	if err != nil {
		t.Fatalf("DecodeDlgAnswer: %v", err)
	}
	if got.MessageID != 1842 || got.Answer != 1 || got.RequesterID != 12345 {
		t.Fatalf("DecodeDlgAnswer() = %+v, want {MessageID:1842 Answer:1 RequesterID:12345}", got)
	}
}

func TestDecodeDlgAnswerShort(t *testing.T) {
	if _, err := DecodeDlgAnswer([]byte{OpcodeDlgAnswer, 0x01}); err == nil {
		t.Fatal("DecodeDlgAnswer() error = nil, want short-payload error")
	}
}
