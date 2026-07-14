package clientpackets

import "testing"

func TestDecodeRequestDropItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestDropItem,
		0xf4, 0x01, 0x00, 0x00,
		0x28, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}

	got, err := DecodeRequestDropItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestDropItem: %v", err)
	}
	want := RequestDropItem{ObjectID: 500, Count: 40, X: 46160, Y: 41237, Z: -3534}
	if got != want {
		t.Fatalf("DecodeRequestDropItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestDestroyItem(t *testing.T) {
	payload := []byte{OpcodeRequestDestroyItem, 0xf5, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}

	got, err := DecodeRequestDestroyItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestDestroyItem: %v", err)
	}
	want := RequestDestroyItem{ObjectID: 501, Count: 2}
	if got != want {
		t.Fatalf("DecodeRequestDestroyItem = %+v, want %+v", got, want)
	}
}

func TestDecodeSendTimeCheck(t *testing.T) {
	payload := []byte{OpcodeSendTimeCheck, 0x11, 0x00, 0x00, 0x00, 0x22, 0x00, 0x00, 0x00}

	got, err := DecodeSendTimeCheck(payload)
	if err != nil {
		t.Fatalf("DecodeSendTimeCheck: %v", err)
	}
	want := SendTimeCheck{RequestID: 17, ResponseID: 34}
	if got != want {
		t.Fatalf("DecodeSendTimeCheck = %+v, want %+v", got, want)
	}
}

func TestDecodeItemOpsShort(t *testing.T) {
	if _, err := DecodeRequestDropItem([]byte{OpcodeRequestDropItem, 1}); err == nil {
		t.Fatal("DecodeRequestDropItem: want error on short payload")
	}
	if _, err := DecodeRequestDestroyItem([]byte{OpcodeRequestDestroyItem, 1}); err == nil {
		t.Fatal("DecodeRequestDestroyItem: want error on short payload")
	}
	if _, err := DecodeSendTimeCheck([]byte{OpcodeSendTimeCheck, 1}); err == nil {
		t.Fatal("DecodeSendTimeCheck: want error on short payload")
	}
}
