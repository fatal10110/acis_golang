package clientpackets

import "testing"

func TestDecodeRequestChangeMoveType(t *testing.T) {
	payload := []byte{OpcodeRequestChangeMoveType, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeRequestChangeMoveType(payload)
	if err != nil {
		t.Fatalf("DecodeRequestChangeMoveType: %v", err)
	}
	if got != (RequestChangeMoveType{Run: true}) {
		t.Fatalf("DecodeRequestChangeMoveType = %+v", got)
	}
}

func TestDecodeRequestChangeWaitType(t *testing.T) {
	payload := []byte{OpcodeRequestChangeWaitType, 0x00, 0x00, 0x00, 0x00}

	got, err := DecodeRequestChangeWaitType(payload)
	if err != nil {
		t.Fatalf("DecodeRequestChangeWaitType: %v", err)
	}
	if got != (RequestChangeWaitType{Stand: false}) {
		t.Fatalf("DecodeRequestChangeWaitType = %+v", got)
	}
}

func TestDecodeRequestSocialAction(t *testing.T) {
	payload := []byte{OpcodeRequestSocialAction, 0x0d, 0x00, 0x00, 0x00}

	got, err := DecodeRequestSocialAction(payload)
	if err != nil {
		t.Fatalf("DecodeRequestSocialAction: %v", err)
	}
	if got != (RequestSocialAction{ActionID: 13}) {
		t.Fatalf("DecodeRequestSocialAction = %+v", got)
	}
}

func TestDecodeStanceAndSocialShort(t *testing.T) {
	if _, err := DecodeRequestChangeMoveType([]byte{OpcodeRequestChangeMoveType, 1}); err == nil {
		t.Fatal("DecodeRequestChangeMoveType: want error on short payload")
	}
	if _, err := DecodeRequestChangeWaitType([]byte{OpcodeRequestChangeWaitType, 1}); err == nil {
		t.Fatal("DecodeRequestChangeWaitType: want error on short payload")
	}
	if _, err := DecodeRequestSocialAction([]byte{OpcodeRequestSocialAction, 1}); err == nil {
		t.Fatal("DecodeRequestSocialAction: want error on short payload")
	}
}
