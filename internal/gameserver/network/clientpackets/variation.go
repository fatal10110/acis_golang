package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const (
	requestConfirmTargetItemSize  = 2 + 4
	requestConfirmRefinerItemSize = 2 + 2*4
	requestConfirmGemStoneSize    = 2 + 4*4
	requestConfirmCancelItemSize  = 2 + 4
)

// RequestConfirmTargetItem asks the server to validate an item for
// augmentation.
type RequestConfirmTargetItem struct {
	ObjectID int32
}

// DecodeRequestConfirmTargetItem parses a raw extended
// RequestConfirmTargetItem payload (opcode byte included).
func DecodeRequestConfirmTargetItem(payload []byte) (RequestConfirmTargetItem, error) {
	r, err := newVariationReader(payload, "RequestConfirmTargetItem", OpcodeRequestConfirmTargetItem, requestConfirmTargetItemSize)
	if err != nil {
		return RequestConfirmTargetItem{}, err
	}
	req := RequestConfirmTargetItem{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestConfirmTargetItem{}, fmt.Errorf("clientpackets: RequestConfirmTargetItem: %w", err)
	}
	return req, nil
}

// RequestConfirmRefinerItem asks the server to validate a life-stone item
// for the selected augmentation target.
type RequestConfirmRefinerItem struct {
	TargetObjectID  int32
	RefinerObjectID int32
}

// DecodeRequestConfirmRefinerItem parses a raw extended
// RequestConfirmRefinerItem payload (opcode byte included).
func DecodeRequestConfirmRefinerItem(payload []byte) (RequestConfirmRefinerItem, error) {
	r, err := newVariationReader(payload, "RequestConfirmRefinerItem", OpcodeRequestConfirmRefinerItem, requestConfirmRefinerItemSize)
	if err != nil {
		return RequestConfirmRefinerItem{}, err
	}
	req := RequestConfirmRefinerItem{
		TargetObjectID:  r.ReadInt32(),
		RefinerObjectID: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestConfirmRefinerItem{}, fmt.Errorf("clientpackets: RequestConfirmRefinerItem: %w", err)
	}
	return req, nil
}

// RequestConfirmGemStone asks the server to validate the gemstone stack for
// the selected augmentation target and life stone.
type RequestConfirmGemStone struct {
	TargetObjectID   int32
	RefinerObjectID  int32
	GemstoneObjectID int32
	GemstoneCount    int32
}

// DecodeRequestConfirmGemStone parses a raw extended
// RequestConfirmGemStone payload (opcode byte included).
func DecodeRequestConfirmGemStone(payload []byte) (RequestConfirmGemStone, error) {
	r, err := newVariationReader(payload, "RequestConfirmGemStone", OpcodeRequestConfirmGemStone, requestConfirmGemStoneSize)
	if err != nil {
		return RequestConfirmGemStone{}, err
	}
	req := RequestConfirmGemStone{
		TargetObjectID:   r.ReadInt32(),
		RefinerObjectID:  r.ReadInt32(),
		GemstoneObjectID: r.ReadInt32(),
		GemstoneCount:    r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestConfirmGemStone{}, fmt.Errorf("clientpackets: RequestConfirmGemStone: %w", err)
	}
	return req, nil
}

// RequestConfirmCancelItem asks the server to validate an augmented item for
// augmentation removal.
type RequestConfirmCancelItem struct {
	ObjectID int32
}

// DecodeRequestConfirmCancelItem parses a raw extended
// RequestConfirmCancelItem payload (opcode byte included).
func DecodeRequestConfirmCancelItem(payload []byte) (RequestConfirmCancelItem, error) {
	r, err := newVariationReader(payload, "RequestConfirmCancelItem", OpcodeRequestConfirmCancelItem, requestConfirmCancelItemSize)
	if err != nil {
		return RequestConfirmCancelItem{}, err
	}
	req := RequestConfirmCancelItem{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestConfirmCancelItem{}, fmt.Errorf("clientpackets: RequestConfirmCancelItem: %w", err)
	}
	return req, nil
}

func newVariationReader(payload []byte, name string, opcode uint16, size int) (*wire.Reader, error) {
	r := newReader(payload)
	if r.Remaining() < size {
		return nil, fmt.Errorf("clientpackets: %s: need %d bytes, got %d", name, size, r.Remaining())
	}
	if second := r.ReadUint16(); second != opcode {
		return nil, fmt.Errorf("clientpackets: %s: extended opcode %#x", name, second)
	}
	return r, nil
}
