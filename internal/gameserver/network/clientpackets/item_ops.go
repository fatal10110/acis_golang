package clientpackets

import "fmt"

const (
	requestDropItemSize     = 5 * 4
	requestDestroyItemSize  = 2 * 4
	requestCrystallizeSize  = 2 * 4
	requestEnchantItemSize  = 4
	requestPetUseItemSize   = 4
	requestGiveItemToPet    = 2 * 4
	requestGetItemFromPet   = 3 * 4
	requestPetGetItem       = 4
	sendTimeCheckSize       = 2 * 4
	requestAutoSoulShotSize = 2 + 2*4
)

// RequestDropItem asks the server to drop an inventory item stack into the
// world at the requested coordinates.
type RequestDropItem struct {
	ObjectID int32
	Count    int32
	X        int32
	Y        int32
	Z        int32
}

// DecodeRequestDropItem parses a raw RequestDropItem payload (opcode byte
// included).
func DecodeRequestDropItem(payload []byte) (RequestDropItem, error) {
	r := newReader(payload)
	if r.Remaining() < requestDropItemSize {
		return RequestDropItem{}, fmt.Errorf("clientpackets: RequestDropItem: need %d bytes, got %d", requestDropItemSize, r.Remaining())
	}
	req := RequestDropItem{
		ObjectID: r.ReadInt32(),
		Count:    r.ReadInt32(),
		X:        r.ReadInt32(),
		Y:        r.ReadInt32(),
		Z:        r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestDropItem{}, fmt.Errorf("clientpackets: RequestDropItem: %w", err)
	}
	return req, nil
}

// RequestDestroyItem asks the server to destroy an inventory item stack.
type RequestDestroyItem struct {
	ObjectID int32
	Count    int32
}

// DecodeRequestDestroyItem parses a raw RequestDestroyItem payload (opcode
// byte included).
func DecodeRequestDestroyItem(payload []byte) (RequestDestroyItem, error) {
	r := newReader(payload)
	if r.Remaining() < requestDestroyItemSize {
		return RequestDestroyItem{}, fmt.Errorf("clientpackets: RequestDestroyItem: need %d bytes, got %d", requestDestroyItemSize, r.Remaining())
	}
	req := RequestDestroyItem{
		ObjectID: r.ReadInt32(),
		Count:    r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestDestroyItem{}, fmt.Errorf("clientpackets: RequestDestroyItem: %w", err)
	}
	return req, nil
}

// RequestCrystallizeItem asks the server to destroy an inventory item and
// grant its crystal reward.
type RequestCrystallizeItem struct {
	ObjectID int32
	Count    int32
}

// DecodeRequestCrystallizeItem parses a raw RequestCrystallizeItem payload
// (opcode byte included).
func DecodeRequestCrystallizeItem(payload []byte) (RequestCrystallizeItem, error) {
	r := newReader(payload)
	if r.Remaining() < requestCrystallizeSize {
		return RequestCrystallizeItem{}, fmt.Errorf("clientpackets: RequestCrystallizeItem: need %d bytes, got %d", requestCrystallizeSize, r.Remaining())
	}
	req := RequestCrystallizeItem{
		ObjectID: r.ReadInt32(),
		Count:    r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestCrystallizeItem{}, fmt.Errorf("clientpackets: RequestCrystallizeItem: %w", err)
	}
	return req, nil
}

// RequestEnchantItem asks the server to enchant the selected inventory item
// with the scroll already opened by UseItem.
type RequestEnchantItem struct {
	ObjectID int32
}

// DecodeRequestEnchantItem parses a raw RequestEnchantItem payload (opcode
// byte included).
func DecodeRequestEnchantItem(payload []byte) (RequestEnchantItem, error) {
	r := newReader(payload)
	if r.Remaining() < requestEnchantItemSize {
		return RequestEnchantItem{}, fmt.Errorf("clientpackets: RequestEnchantItem: need %d bytes, got %d", requestEnchantItemSize, r.Remaining())
	}
	req := RequestEnchantItem{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestEnchantItem{}, fmt.Errorf("clientpackets: RequestEnchantItem: %w", err)
	}
	return req, nil
}

// RequestPetUseItem asks the server to use or toggle a pet inventory item.
type RequestPetUseItem struct {
	ObjectID int32
}

// DecodeRequestPetUseItem parses a raw RequestPetUseItem payload (opcode
// byte included).
func DecodeRequestPetUseItem(payload []byte) (RequestPetUseItem, error) {
	r := newReader(payload)
	if r.Remaining() < requestPetUseItemSize {
		return RequestPetUseItem{}, fmt.Errorf("clientpackets: RequestPetUseItem: need %d bytes, got %d", requestPetUseItemSize, r.Remaining())
	}
	req := RequestPetUseItem{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPetUseItem{}, fmt.Errorf("clientpackets: RequestPetUseItem: %w", err)
	}
	return req, nil
}

// RequestGiveItemToPet asks the server to move a player inventory item to
// the active pet.
type RequestGiveItemToPet struct {
	ObjectID int32
	Count    int32
}

// DecodeRequestGiveItemToPet parses a raw RequestGiveItemToPet payload
// (opcode byte included).
func DecodeRequestGiveItemToPet(payload []byte) (RequestGiveItemToPet, error) {
	r := newReader(payload)
	if r.Remaining() < requestGiveItemToPet {
		return RequestGiveItemToPet{}, fmt.Errorf("clientpackets: RequestGiveItemToPet: need %d bytes, got %d", requestGiveItemToPet, r.Remaining())
	}
	req := RequestGiveItemToPet{
		ObjectID: r.ReadInt32(),
		Count:    r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestGiveItemToPet{}, fmt.Errorf("clientpackets: RequestGiveItemToPet: %w", err)
	}
	return req, nil
}

// RequestGetItemFromPet asks the server to move a pet inventory item back
// to the owner.
type RequestGetItemFromPet struct {
	ObjectID int32
	Count    int32
	Unknown  int32
}

// DecodeRequestGetItemFromPet parses a raw RequestGetItemFromPet payload
// (opcode byte included).
func DecodeRequestGetItemFromPet(payload []byte) (RequestGetItemFromPet, error) {
	r := newReader(payload)
	if r.Remaining() < requestGetItemFromPet {
		return RequestGetItemFromPet{}, fmt.Errorf("clientpackets: RequestGetItemFromPet: need %d bytes, got %d", requestGetItemFromPet, r.Remaining())
	}
	req := RequestGetItemFromPet{
		ObjectID: r.ReadInt32(),
		Count:    r.ReadInt32(),
		Unknown:  r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestGetItemFromPet{}, fmt.Errorf("clientpackets: RequestGetItemFromPet: %w", err)
	}
	return req, nil
}

// RequestPetGetItem asks the active pet to pick up a world item.
type RequestPetGetItem struct {
	ObjectID int32
}

// DecodeRequestPetGetItem parses a raw RequestPetGetItem payload (opcode
// byte included).
func DecodeRequestPetGetItem(payload []byte) (RequestPetGetItem, error) {
	r := newReader(payload)
	if r.Remaining() < requestPetGetItem {
		return RequestPetGetItem{}, fmt.Errorf("clientpackets: RequestPetGetItem: need %d bytes, got %d", requestPetGetItem, r.Remaining())
	}
	req := RequestPetGetItem{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPetGetItem{}, fmt.Errorf("clientpackets: RequestPetGetItem: %w", err)
	}
	return req, nil
}

// SendTimeCheck carries a client timing probe response. The Interlude server
// accepts and ignores it.
type SendTimeCheck struct {
	RequestID  int32
	ResponseID int32
}

// DecodeSendTimeCheck parses a raw SendTimeCheck payload (opcode byte
// included).
func DecodeSendTimeCheck(payload []byte) (SendTimeCheck, error) {
	r := newReader(payload)
	if r.Remaining() < sendTimeCheckSize {
		return SendTimeCheck{}, fmt.Errorf("clientpackets: SendTimeCheck: need %d bytes, got %d", sendTimeCheckSize, r.Remaining())
	}
	req := SendTimeCheck{
		RequestID:  r.ReadInt32(),
		ResponseID: r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return SendTimeCheck{}, fmt.Errorf("clientpackets: SendTimeCheck: %w", err)
	}
	return req, nil
}

// RequestAutoSoulShot asks the server to toggle automatic use for a shot
// item. Type is 1 to enable and 0 to disable.
type RequestAutoSoulShot struct {
	ItemID int32
	Type   int32
}

// DecodeRequestAutoSoulShot parses a raw extended RequestAutoSoulShot payload
// (opcode byte included).
func DecodeRequestAutoSoulShot(payload []byte) (RequestAutoSoulShot, error) {
	r := newReader(payload)
	if r.Remaining() < requestAutoSoulShotSize {
		return RequestAutoSoulShot{}, fmt.Errorf("clientpackets: RequestAutoSoulShot: need %d bytes, got %d", requestAutoSoulShotSize, r.Remaining())
	}
	if second := r.ReadUint16(); second != OpcodeRequestAutoSoulShot {
		return RequestAutoSoulShot{}, fmt.Errorf("clientpackets: RequestAutoSoulShot: extended opcode %#x", second)
	}
	req := RequestAutoSoulShot{
		ItemID: r.ReadInt32(),
		Type:   r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return RequestAutoSoulShot{}, fmt.Errorf("clientpackets: RequestAutoSoulShot: %w", err)
	}
	return req, nil
}
