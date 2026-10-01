package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Symbol maker window opcodes.
const (
	OpcodeRequestHennaItemList    = 0xba
	OpcodeRequestHennaItemInfo    = 0xbb
	OpcodeRequestHennaEquip       = 0xbc
	OpcodeRequestHennaUnequipList = 0xbd
	OpcodeRequestHennaUnequipInfo = 0xbe
	OpcodeRequestHennaUnequip     = 0xbf
)

// RequestHennaItemList asks for the draw window again. Its one int32 has no
// known meaning and is read and ignored.
type RequestHennaItemList struct{}

// DecodeRequestHennaItemList parses a raw RequestHennaItemList payload
// (opcode byte included).
func DecodeRequestHennaItemList(payload []byte) (RequestHennaItemList, error) {
	_, err := decodeHennaInt(payload, "RequestHennaItemList")
	return RequestHennaItemList{}, err
}

// RequestHennaUnequipList asks for the deletion window again. Its one int32
// has no known meaning and is read and ignored.
type RequestHennaUnequipList struct{}

// DecodeRequestHennaUnequipList parses a raw RequestHennaUnequipList
// payload (opcode byte included).
func DecodeRequestHennaUnequipList(payload []byte) (RequestHennaUnequipList, error) {
	_, err := decodeHennaInt(payload, "RequestHennaUnequipList")
	return RequestHennaUnequipList{}, err
}

// RequestHennaSymbol is a symbol maker request naming one symbol:
// RequestHennaItemInfo, RequestHennaEquip, RequestHennaUnequipInfo and
// RequestHennaUnequip all carry just its id.
type RequestHennaSymbol struct {
	SymbolID int32
}

// DecodeRequestHennaItemInfo parses a raw RequestHennaItemInfo payload
// (opcode byte included).
func DecodeRequestHennaItemInfo(payload []byte) (RequestHennaSymbol, error) {
	id, err := decodeHennaInt(payload, "RequestHennaItemInfo")
	return RequestHennaSymbol{SymbolID: id}, err
}

// DecodeRequestHennaEquip parses a raw RequestHennaEquip payload (opcode
// byte included).
func DecodeRequestHennaEquip(payload []byte) (RequestHennaSymbol, error) {
	id, err := decodeHennaInt(payload, "RequestHennaEquip")
	return RequestHennaSymbol{SymbolID: id}, err
}

// DecodeRequestHennaUnequipInfo parses a raw RequestHennaUnequipInfo
// payload (opcode byte included).
func DecodeRequestHennaUnequipInfo(payload []byte) (RequestHennaSymbol, error) {
	id, err := decodeHennaInt(payload, "RequestHennaUnequipInfo")
	return RequestHennaSymbol{SymbolID: id}, err
}

// DecodeRequestHennaUnequip parses a raw RequestHennaUnequip payload
// (opcode byte included).
func DecodeRequestHennaUnequip(payload []byte) (RequestHennaSymbol, error) {
	id, err := decodeHennaInt(payload, "RequestHennaUnequip")
	return RequestHennaSymbol{SymbolID: id}, err
}

// decodeHennaInt reads the single int32 every symbol maker request carries.
func decodeHennaInt(payload []byte, name string) (int32, error) {
	r := newReader(payload)
	if r.Remaining() < 4 {
		return 0, fmt.Errorf("clientpackets: %s: need 4 bytes, got %d: %w", name, r.Remaining(), wire.ErrShortPacket)
	}
	v := r.ReadInt32()
	if err := r.Err(); err != nil {
		return 0, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return v, nil
}
