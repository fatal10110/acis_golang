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

func TestDecodeRequestCrystallizeItem(t *testing.T) {
	payload := []byte{OpcodeRequestCrystallizeItem, 0xf6, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeRequestCrystallizeItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestCrystallizeItem: %v", err)
	}
	want := RequestCrystallizeItem{ObjectID: 502, Count: 1}
	if got != want {
		t.Fatalf("DecodeRequestCrystallizeItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestEnchantItem(t *testing.T) {
	payload := []byte{OpcodeRequestEnchantItem, 0xf7, 0x01, 0x00, 0x00}

	got, err := DecodeRequestEnchantItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestEnchantItem: %v", err)
	}
	want := RequestEnchantItem{ObjectID: 503}
	if got != want {
		t.Fatalf("DecodeRequestEnchantItem = %+v, want %+v", got, want)
	}
}

func TestDecodePetItemRequests(t *testing.T) {
	use, err := DecodeRequestPetUseItem([]byte{OpcodeRequestPetUseItem, 0x21, 0x03, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestPetUseItem: %v", err)
	}
	if use != (RequestPetUseItem{ObjectID: 801}) {
		t.Fatalf("DecodeRequestPetUseItem = %+v, want ObjectID 801", use)
	}

	give, err := DecodeRequestGiveItemToPet([]byte{OpcodeRequestGiveItemToPet, 0x22, 0x03, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestGiveItemToPet: %v", err)
	}
	if give != (RequestGiveItemToPet{ObjectID: 802, Count: 5}) {
		t.Fatalf("DecodeRequestGiveItemToPet = %+v, want ObjectID 802 Count 5", give)
	}

	take, err := DecodeRequestGetItemFromPet([]byte{OpcodeRequestGetItemFromPet, 0x23, 0x03, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff})
	if err != nil {
		t.Fatalf("DecodeRequestGetItemFromPet: %v", err)
	}
	if take != (RequestGetItemFromPet{ObjectID: 803, Count: 6, Unknown: -1}) {
		t.Fatalf("DecodeRequestGetItemFromPet = %+v, want ObjectID 803 Count 6 Unknown -1", take)
	}

	pickup, err := DecodeRequestPetGetItem([]byte{OpcodeRequestPetGetItem, 0x24, 0x03, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestPetGetItem: %v", err)
	}
	if pickup != (RequestPetGetItem{ObjectID: 804}) {
		t.Fatalf("DecodeRequestPetGetItem = %+v, want ObjectID 804", pickup)
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

func TestDecodeRequestAutoSoulShot(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x05, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAutoSoulShot(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAutoSoulShot: %v", err)
	}
	want := RequestAutoSoulShot{ItemID: 1463, Type: 1}
	if got != want {
		t.Fatalf("DecodeRequestAutoSoulShot = %+v, want %+v", got, want)
	}
}

func TestDecodeItemOpsShort(t *testing.T) {
	if _, err := DecodeRequestDropItem([]byte{OpcodeRequestDropItem, 1}); err == nil {
		t.Fatal("DecodeRequestDropItem: want error on short payload")
	}
	if _, err := DecodeRequestDestroyItem([]byte{OpcodeRequestDestroyItem, 1}); err == nil {
		t.Fatal("DecodeRequestDestroyItem: want error on short payload")
	}
	if _, err := DecodeRequestCrystallizeItem([]byte{OpcodeRequestCrystallizeItem, 1}); err == nil {
		t.Fatal("DecodeRequestCrystallizeItem: want error on short payload")
	}
	if _, err := DecodeRequestEnchantItem([]byte{OpcodeRequestEnchantItem, 1}); err == nil {
		t.Fatal("DecodeRequestEnchantItem: want error on short payload")
	}
	if _, err := DecodeRequestPetUseItem([]byte{OpcodeRequestPetUseItem, 1}); err == nil {
		t.Fatal("DecodeRequestPetUseItem: want error on short payload")
	}
	if _, err := DecodeRequestGiveItemToPet([]byte{OpcodeRequestGiveItemToPet, 1}); err == nil {
		t.Fatal("DecodeRequestGiveItemToPet: want error on short payload")
	}
	if _, err := DecodeRequestGetItemFromPet([]byte{OpcodeRequestGetItemFromPet, 1}); err == nil {
		t.Fatal("DecodeRequestGetItemFromPet: want error on short payload")
	}
	if _, err := DecodeRequestPetGetItem([]byte{OpcodeRequestPetGetItem, 1}); err == nil {
		t.Fatal("DecodeRequestPetGetItem: want error on short payload")
	}
	if _, err := DecodeSendTimeCheck([]byte{OpcodeSendTimeCheck, 1}); err == nil {
		t.Fatal("DecodeSendTimeCheck: want error on short payload")
	}
	if _, err := DecodeRequestAutoSoulShot([]byte{OpcodeExtended, 0x05, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestAutoSoulShot: want error on short payload")
	}
	if _, err := DecodeRequestAutoSoulShot([]byte{OpcodeExtended, 0x08, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestAutoSoulShot: want error on wrong extended opcode")
	}
}
