package clientpackets

import "testing"

func TestDecodeAction(t *testing.T) {
	payload := []byte{
		OpcodeAction,
		0x39, 0x30, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x01,
	}

	got, err := DecodeAction(payload)
	if err != nil {
		t.Fatalf("DecodeAction: %v", err)
	}
	want := Action{ObjectID: 12345, OriginX: 46160, OriginY: 41237, OriginZ: -3534, Shift: true}
	if got != want {
		t.Fatalf("DecodeAction = %+v, want %+v", got, want)
	}
}

func TestDecodeAttackRequest(t *testing.T) {
	payload := []byte{
		OpcodeAttackRequest,
		0x39, 0x30, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00,
	}

	got, err := DecodeAttackRequest(payload)
	if err != nil {
		t.Fatalf("DecodeAttackRequest: %v", err)
	}
	want := AttackRequest{ObjectID: 12345, OriginX: 46160, OriginY: 41237, OriginZ: -3534}
	if got != want {
		t.Fatalf("DecodeAttackRequest = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestTargetCancel(t *testing.T) {
	payload := []byte{OpcodeRequestTargetCancel, 0x01, 0x00}

	got, err := DecodeRequestTargetCancel(payload)
	if err != nil {
		t.Fatalf("DecodeRequestTargetCancel: %v", err)
	}
	if got != (RequestTargetCancel{Unselect: 1}) {
		t.Fatalf("DecodeRequestTargetCancel = %+v", got)
	}
}

func TestDecodeTargetActionShort(t *testing.T) {
	if _, err := DecodeAction([]byte{OpcodeAction, 1, 2}); err == nil {
		t.Fatal("DecodeAction: want error on short payload")
	}
	if _, err := DecodeAttackRequest([]byte{OpcodeAttackRequest, 1, 2}); err == nil {
		t.Fatal("DecodeAttackRequest: want error on short payload")
	}
	if _, err := DecodeRequestTargetCancel([]byte{OpcodeRequestTargetCancel}); err == nil {
		t.Fatal("DecodeRequestTargetCancel: want error on short payload")
	}
}
