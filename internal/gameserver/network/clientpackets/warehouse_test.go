package clientpackets

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

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

// TestDecodeRequestPackageSendCountAboveMaxIsNotShortPacket proves a
// count exceeding maxItemInPacket (mirroring Config.MAX_ITEM_IN_PACKET,
// whose readImpl() guard returns silently before any row read, so it can
// never throw BufferUnderflowException) is a plain validation error, not
// classified as a buffer-underflow-equivalent wire.ErrShortPacket -- even
// though its trailing byte count is also short for that count, matching
// the reference's guard order (count bound checked first, silently).
func TestDecodeRequestPackageSendCountAboveMaxIsNotShortPacket(t *testing.T) {
	payload := []byte{
		OpcodeRequestPackageSend,
		0x78, 0x56, 0x34, 0x12,
		0x65, 0x00, 0x00, 0x00, // count = 101, exceeds maxItemInPacket (100)
	}

	_, err := DecodeRequestPackageSend(payload)
	if err == nil {
		t.Fatal("DecodeRequestPackageSend: want error for count above max")
	}
	if errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeRequestPackageSend() error = %v, want a non-short-packet validation error", err)
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
