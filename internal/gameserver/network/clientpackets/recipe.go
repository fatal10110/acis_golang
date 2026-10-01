package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Recipe book opcodes.
const (
	OpcodeRequestRecipeBookOpen     = 0xac
	OpcodeRequestRecipeBookDestroy  = 0xad
	OpcodeRequestRecipeItemMakeInfo = 0xae
	OpcodeRequestRecipeItemMakeSelf = 0xaf
)

// RequestRecipeBookOpen asks for the dwarven or common recipe book page.
type RequestRecipeBookOpen struct {
	Dwarven bool
}

// DecodeRequestRecipeBookOpen parses a raw RequestRecipeBookOpen payload
// (opcode byte included). A book type of 0 names the dwarven page; any
// other value the common one.
func DecodeRequestRecipeBookOpen(payload []byte) (RequestRecipeBookOpen, error) {
	typ, err := decodeRecipeInt(payload, "RequestRecipeBookOpen")
	if err != nil {
		return RequestRecipeBookOpen{}, err
	}
	return RequestRecipeBookOpen{Dwarven: typ == 0}, nil
}

// RequestRecipeBookDestroy asks to delete RecipeID from the recipe book.
type RequestRecipeBookDestroy struct {
	RecipeID int32
}

// DecodeRequestRecipeBookDestroy parses a raw RequestRecipeBookDestroy
// payload (opcode byte included).
func DecodeRequestRecipeBookDestroy(payload []byte) (RequestRecipeBookDestroy, error) {
	id, err := decodeRecipeInt(payload, "RequestRecipeBookDestroy")
	return RequestRecipeBookDestroy{RecipeID: id}, err
}

// RequestRecipeItemMakeInfo asks for RecipeID's craft window.
type RequestRecipeItemMakeInfo struct {
	RecipeID int32
}

// DecodeRequestRecipeItemMakeInfo parses a raw RequestRecipeItemMakeInfo
// payload (opcode byte included).
func DecodeRequestRecipeItemMakeInfo(payload []byte) (RequestRecipeItemMakeInfo, error) {
	id, err := decodeRecipeInt(payload, "RequestRecipeItemMakeInfo")
	return RequestRecipeItemMakeInfo{RecipeID: id}, err
}

// RequestRecipeItemMakeSelf asks to craft RecipeID for oneself.
type RequestRecipeItemMakeSelf struct {
	RecipeID int32
}

// DecodeRequestRecipeItemMakeSelf parses a raw RequestRecipeItemMakeSelf
// payload (opcode byte included).
func DecodeRequestRecipeItemMakeSelf(payload []byte) (RequestRecipeItemMakeSelf, error) {
	id, err := decodeRecipeInt(payload, "RequestRecipeItemMakeSelf")
	return RequestRecipeItemMakeSelf{RecipeID: id}, err
}

// decodeRecipeInt reads the single int32 every recipe book request
// carries.
func decodeRecipeInt(payload []byte, name string) (int32, error) {
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
