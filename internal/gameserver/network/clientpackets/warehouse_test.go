package clientpackets

import "testing"

func TestDecodeWarehouseItemBatchPackets(t *testing.T) {
	payload := []byte{
		OpcodeSendWarehouseDeposit,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	deposit, err := DecodeSendWarehouseDepositList(payload)
	if err != nil {
		t.Fatalf("DecodeSendWarehouseDepositList: %v", err)
	}
	withdraw, err := DecodeSendWarehouseWithdrawList(append([]byte{OpcodeSendWarehouseWithdraw}, payload[1:]...))
	if err != nil {
		t.Fatalf("DecodeSendWarehouseWithdrawList: %v", err)
	}

	want := []ItemRequest{{ObjectID: 500, Count: 3}, {ObjectID: 501, Count: 1}}
	if !sameItemRequests(deposit.Items, want) {
		t.Fatalf("deposit items = %+v, want %+v", deposit.Items, want)
	}
	if !sameItemRequests(withdraw.Items, want) {
		t.Fatalf("withdraw items = %+v, want %+v", withdraw.Items, want)
	}
}

func TestDecodeWarehouseItemBatchRejectsMalformedPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"zero count", []byte{OpcodeSendWarehouseDeposit, 0, 0, 0, 0}},
		{"trailing byte", []byte{OpcodeSendWarehouseDeposit, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0}},
		{"bad object id", []byte{OpcodeSendWarehouseDeposit, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0}},
		{"negative count", []byte{OpcodeSendWarehouseDeposit, 1, 0, 0, 0, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeSendWarehouseDepositList(tt.payload); err == nil {
				t.Fatal("DecodeSendWarehouseDepositList: want error")
			}
		})
	}
}

func TestDecodeRequestPackageSendableItemList(t *testing.T) {
	payload := []byte{OpcodeRequestPackageItemList, 0x78, 0x56, 0x34, 0x12}

	got, err := DecodeRequestPackageSendableItemList(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSendableItemList: %v", err)
	}
	if got.ObjectID != 0x12345678 {
		t.Fatalf("ObjectID = %#x, want 0x12345678", got.ObjectID)
	}
}

func TestDecodeRequestPackageSend(t *testing.T) {
	payload := []byte{
		OpcodeRequestPackageSend,
		0x78, 0x56, 0x34, 0x12,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestPackageSend(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSend: %v", err)
	}
	want := RequestPackageSend{ObjectID: 0x12345678, Items: []ItemRequest{{ObjectID: 500, Count: 3}, {ObjectID: 501, Count: 1}}}
	if got.ObjectID != want.ObjectID || !sameItemRequests(got.Items, want.Items) {
		t.Fatalf("DecodeRequestPackageSend = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestPackageSendAllowsEmptyList(t *testing.T) {
	payload := []byte{OpcodeRequestPackageSend, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0}

	got, err := DecodeRequestPackageSend(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSend: %v", err)
	}
	if got.ObjectID != 0x12345678 || len(got.Items) != 0 {
		t.Fatalf("DecodeRequestPackageSend = %+v, want object id with no items", got)
	}
}

func TestDecodeRequestPackageSendRejectsMalformedPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"short header", []byte{OpcodeRequestPackageSend, 1}},
		{"negative count", []byte{OpcodeRequestPackageSend, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}},
		{"short item", []byte{OpcodeRequestPackageSend, 1, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeRequestPackageSend(tt.payload); err == nil {
				t.Fatal("DecodeRequestPackageSend: want error")
			}
		})
	}
}

func sameItemRequests(a, b []ItemRequest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
