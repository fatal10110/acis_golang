package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const itemRequestSize = 2 * 4

// maxItemInPacket mirrors Config.MAX_ITEM_IN_PACKET, the larger of the
// (file-configurable, but not yet plumbed here) MaximumSlotsForNoDwarf/
// MaximumSlotsForDwarf player.properties defaults (80/100).
const maxItemInPacket = 100

// ItemRequest identifies one item object and the requested unit count.
type ItemRequest struct {
	ObjectID int32
	Count    int32
}

// SendWarehouseDepositList asks to move inventory items into the active
// warehouse.
type SendWarehouseDepositList struct {
	Items []ItemRequest
}

// SendWarehouseWithdrawList asks to move active-warehouse items into the
// inventory.
type SendWarehouseWithdrawList struct {
	Items []ItemRequest
}

// RequestPackageSendableItemList asks which inventory items can be sent as
// freight to ObjectID.
type RequestPackageSendableItemList struct {
	ObjectID int32
}

// RequestPackageSend asks to send inventory items as freight to ObjectID.
type RequestPackageSend struct {
	ObjectID int32
	Items    []ItemRequest
}

// DecodeSendWarehouseDepositList parses a raw SendWarehouseDepositList
// payload (opcode byte included).
func DecodeSendWarehouseDepositList(payload []byte) (SendWarehouseDepositList, error) {
	items, err := decodeExactItemRequestBatch(payload, "SendWarehouseDepositList")
	if err != nil {
		return SendWarehouseDepositList{}, err
	}
	return SendWarehouseDepositList{Items: items}, nil
}

// DecodeSendWarehouseWithdrawList parses a raw SendWarehouseWithdrawList
// payload (opcode byte included).
func DecodeSendWarehouseWithdrawList(payload []byte) (SendWarehouseWithdrawList, error) {
	items, err := decodeExactItemRequestBatch(payload, "SendWarehouseWithdrawList")
	if err != nil {
		return SendWarehouseWithdrawList{}, err
	}
	return SendWarehouseWithdrawList{Items: items}, nil
}

// DecodeRequestPackageSendableItemList parses a raw
// RequestPackageSendableItemList payload (opcode byte included).
func DecodeRequestPackageSendableItemList(payload []byte) (RequestPackageSendableItemList, error) {
	r := newReader(payload)
	if r.Remaining() < 4 {
		return RequestPackageSendableItemList{}, fmt.Errorf("clientpackets: RequestPackageSendableItemList: need 4 bytes, got %d: %w", r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestPackageSendableItemList{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPackageSendableItemList{}, fmt.Errorf("clientpackets: RequestPackageSendableItemList: %w", err)
	}
	return req, nil
}

// DecodeRequestPackageSend parses a raw RequestPackageSend payload (opcode
// byte included).
func DecodeRequestPackageSend(payload []byte) (RequestPackageSend, error) {
	r := newReader(payload)
	if r.Remaining() < 8 {
		return RequestPackageSend{}, fmt.Errorf("clientpackets: RequestPackageSend: need 8 bytes, got %d: %w", r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestPackageSend{ObjectID: r.ReadInt32()}
	count := r.ReadInt32()
	// The reference guards the row loop with a silent readImpl() return for
	// count < 0 or count > MAX_ITEM_IN_PACKET, before ever attempting a row
	// read: neither case can throw BufferUnderflowException, so neither
	// counts toward the underflow threshold. Only a count inside that range
	// can genuinely underflow reading its rows below.
	if count < 0 {
		return RequestPackageSend{}, fmt.Errorf("clientpackets: RequestPackageSend: negative item count %d", count)
	}
	if count > maxItemInPacket {
		return RequestPackageSend{}, fmt.Errorf("clientpackets: RequestPackageSend: item count %d exceeds max %d", count, maxItemInPacket)
	}
	if r.Remaining() < int(count)*itemRequestSize {
		return RequestPackageSend{}, fmt.Errorf("clientpackets: RequestPackageSend: need %d item bytes, got %d: %w", int(count)*itemRequestSize, r.Remaining(), wire.ErrShortPacket)
	}
	req.Items = make([]ItemRequest, int(count))
	for i := range req.Items {
		req.Items[i] = ItemRequest{ObjectID: r.ReadInt32(), Count: r.ReadInt32()}
	}
	if err := r.Err(); err != nil {
		return RequestPackageSend{}, fmt.Errorf("clientpackets: RequestPackageSend: %w", err)
	}
	return req, nil
}

func decodeExactItemRequestBatch(payload []byte, name string) ([]ItemRequest, error) {
	r := newReader(payload)
	if r.Remaining() < 4 {
		return nil, fmt.Errorf("clientpackets: %s: need 4 bytes, got %d: %w", name, r.Remaining(), wire.ErrShortPacket)
	}
	count := r.ReadInt32()
	if count <= 0 {
		return nil, fmt.Errorf("clientpackets: %s: invalid item count %d", name, count)
	}
	// A row-count/remaining-length mismatch mirrors the reference's silent
	// readImpl() return (SendWarehouseDepositList/WithdrawList: "count *
	// BATCH_LENGTH != _buf.remaining()"), not a BufferUnderflowException: it
	// never counts toward the underflow threshold, so this stays unwrapped.
	if r.Remaining() != int(count)*itemRequestSize {
		return nil, fmt.Errorf("clientpackets: %s: item bytes = %d, want %d", name, r.Remaining(), int(count)*itemRequestSize)
	}

	items := make([]ItemRequest, int(count))
	for i := range items {
		items[i] = ItemRequest{ObjectID: r.ReadInt32(), Count: r.ReadInt32()}
		if items[i].ObjectID < 1 || items[i].Count < 0 {
			return nil, fmt.Errorf("clientpackets: %s: invalid item request %+v", name, items[i])
		}
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return items, nil
}
