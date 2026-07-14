package clientpackets

import "testing"

func TestDecodeRequestActionUse(t *testing.T) {
	payload := []byte{
		OpcodeRequestActionUse,
		0x34, 0x00, 0x00, 0x00, // action id 52
		0x01, 0x00, 0x00, 0x00, // ctrl
		0x01, // shift
	}

	got, err := DecodeRequestActionUse(payload)
	if err != nil {
		t.Fatalf("DecodeRequestActionUse: %v", err)
	}
	if got != (RequestActionUse{ActionID: 52, CtrlPressed: true, ShiftPressed: true}) {
		t.Fatalf("DecodeRequestActionUse = %+v", got)
	}
}

func TestDecodeRequestActionUse_Short(t *testing.T) {
	if _, err := DecodeRequestActionUse([]byte{OpcodeRequestActionUse, 1, 2}); err == nil {
		t.Fatal("DecodeRequestActionUse: want error on short payload")
	}
}
