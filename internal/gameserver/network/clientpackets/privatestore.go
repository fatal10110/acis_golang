package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Private store and workshop opcodes.
const (
	OpcodeRequestPrivateStoreManageSell = 0x73
	OpcodeSetPrivateStoreListSell       = 0x74
	OpcodeRequestPrivateStoreQuitSell   = 0x76
	OpcodeSetPrivateStoreMsgSell        = 0x77
	OpcodeRequestPrivateStoreBuy        = 0x79
	OpcodeRequestPrivateStoreManageBuy  = 0x90
	OpcodeSetPrivateStoreListBuy        = 0x91
	OpcodeRequestPrivateStoreQuitBuy    = 0x93
	OpcodeSetPrivateStoreMsgBuy         = 0x94
	OpcodeRequestPrivateStoreSell       = 0x96
	OpcodeRequestRecipeShopMessageSet   = 0xb1
	OpcodeRequestRecipeShopListSet      = 0xb2
	OpcodeRequestRecipeShopManageQuit   = 0xb3
	OpcodeRequestRecipeShopMakeInfo     = 0xb5
	OpcodeRequestRecipeShopMakeItem     = 0xb6
	OpcodeRequestRecipeShopManagePrev   = 0xb7
)

const (
	storeSellRowSize        = 3 * 4
	storeBuyRowSize         = 4 + 2 + 2 + 4 + 4
	storePurchaseRowSize    = 3 * 4
	storeSaleRowSize        = 4 + 4 + 2 + 2 + 4 + 4
	recipeShopRowSize       = 2 * 4
	recipeShopMakeInfoSize  = 2 * 4
	recipeShopMakeItemSize  = 3 * 4
	privateStoreHeaderSize  = 4
	privateStoreDealHeader  = 2 * 4
	setPrivateStoreSellHead = 2 * 4
)

// StoreSellRow is one row of SetPrivateStoreListSell.
type StoreSellRow struct {
	ObjectID int32
	Count    int32
	Price    int32
}

// SetPrivateStoreListSell lists items for sale. Items is nil when the
// packet's row count is out of range or disagrees with its length.
type SetPrivateStoreListSell struct {
	Packaged bool
	Items    []StoreSellRow
}

// DecodeSetPrivateStoreListSell parses a raw SetPrivateStoreListSell
// payload (opcode byte included) of at most maxItems rows
// (player.InventorySlots.MaxItemInPacket).
func DecodeSetPrivateStoreListSell(payload []byte, maxItems int) (SetPrivateStoreListSell, error) {
	r := newReader(payload)
	if r.Remaining() < setPrivateStoreSellHead {
		return SetPrivateStoreListSell{}, fmt.Errorf("clientpackets: SetPrivateStoreListSell: need %d bytes, got %d: %w", setPrivateStoreSellHead, r.Remaining(), wire.ErrShortPacket)
	}
	req := SetPrivateStoreListSell{Packaged: r.ReadInt32() == 1}
	count := r.ReadInt32()
	if !storeRowsFit(count, 1, maxItems, storeSellRowSize, r.Remaining()) {
		return req, nil
	}
	req.Items = make([]StoreSellRow, count)
	for i := range req.Items {
		req.Items[i] = StoreSellRow{ObjectID: r.ReadInt32(), Count: r.ReadInt32(), Price: r.ReadInt32()}
	}
	if err := r.Err(); err != nil {
		return SetPrivateStoreListSell{}, fmt.Errorf("clientpackets: SetPrivateStoreListSell: %w", err)
	}
	return req, nil
}

// StoreBuyRow is one row of SetPrivateStoreListBuy.
type StoreBuyRow struct {
	ItemID  int32
	Enchant int
	Count   int32
	Price   int32
}

// SetPrivateStoreListBuy lists items to buy. Items is nil when the packet's
// row count is out of range or disagrees with its length.
type SetPrivateStoreListBuy struct {
	Items []StoreBuyRow
}

// DecodeSetPrivateStoreListBuy parses a raw SetPrivateStoreListBuy payload
// (opcode byte included) of at most maxItems rows.
func DecodeSetPrivateStoreListBuy(payload []byte, maxItems int) (SetPrivateStoreListBuy, error) {
	r := newReader(payload)
	if r.Remaining() < privateStoreHeaderSize {
		return SetPrivateStoreListBuy{}, fmt.Errorf("clientpackets: SetPrivateStoreListBuy: need %d bytes, got %d: %w", privateStoreHeaderSize, r.Remaining(), wire.ErrShortPacket)
	}
	count := r.ReadInt32()
	var req SetPrivateStoreListBuy
	if !storeRowsFit(count, 1, maxItems, storeBuyRowSize, r.Remaining()) {
		return req, nil
	}
	req.Items = make([]StoreBuyRow, count)
	for i := range req.Items {
		row := StoreBuyRow{ItemID: r.ReadInt32(), Enchant: int(r.ReadUint16())}
		r.ReadUint16()
		row.Count = r.ReadInt32()
		row.Price = r.ReadInt32()
		req.Items[i] = row
	}
	if err := r.Err(); err != nil {
		return SetPrivateStoreListBuy{}, fmt.Errorf("clientpackets: SetPrivateStoreListBuy: %w", err)
	}
	return req, nil
}

// StorePurchaseRow is one row of RequestPrivateStoreBuy.
type StorePurchaseRow struct {
	ObjectID int32
	Count    int32
	Price    int32
}

// RequestPrivateStoreBuy buys from StoreID's sell store. Items is nil when
// the row count is out of range or disagrees with the packet's length, or a
// row names an object id or count below 1 or a negative price.
type RequestPrivateStoreBuy struct {
	StoreID int32
	Items   []StorePurchaseRow
}

// DecodeRequestPrivateStoreBuy parses a raw RequestPrivateStoreBuy payload
// (opcode byte included) of at most maxItems rows.
func DecodeRequestPrivateStoreBuy(payload []byte, maxItems int) (RequestPrivateStoreBuy, error) {
	r := newReader(payload)
	if r.Remaining() < privateStoreDealHeader {
		return RequestPrivateStoreBuy{}, fmt.Errorf("clientpackets: RequestPrivateStoreBuy: need %d bytes, got %d: %w", privateStoreDealHeader, r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestPrivateStoreBuy{StoreID: r.ReadInt32()}
	count := r.ReadInt32()
	if !storeRowsFit(count, 1, maxItems, storePurchaseRowSize, r.Remaining()) {
		return req, nil
	}
	items := make([]StorePurchaseRow, count)
	valid := true
	for i := range items {
		row := StorePurchaseRow{ObjectID: r.ReadInt32(), Count: r.ReadInt32(), Price: r.ReadInt32()}
		if row.ObjectID < 1 || row.Count < 1 || row.Price < 0 {
			valid = false
			break
		}
		items[i] = row
	}
	if err := r.Err(); err != nil {
		return RequestPrivateStoreBuy{}, fmt.Errorf("clientpackets: RequestPrivateStoreBuy: %w", err)
	}
	if valid {
		req.Items = items
	}
	return req, nil
}

// StoreSaleRow is one row of RequestPrivateStoreSell.
type StoreSaleRow struct {
	ObjectID int32
	ItemID   int32
	Enchant  int
	Count    int32
	Price    int32
}

// RequestPrivateStoreSell sells into StoreID's buy store. Items is nil when
// the row count is out of range or disagrees with the packet's length, or a
// row names an object id, item id or count below 1 or a negative price.
type RequestPrivateStoreSell struct {
	StoreID int32
	Items   []StoreSaleRow
}

// DecodeRequestPrivateStoreSell parses a raw RequestPrivateStoreSell
// payload (opcode byte included) of at most maxItems rows.
func DecodeRequestPrivateStoreSell(payload []byte, maxItems int) (RequestPrivateStoreSell, error) {
	r := newReader(payload)
	if r.Remaining() < privateStoreDealHeader {
		return RequestPrivateStoreSell{}, fmt.Errorf("clientpackets: RequestPrivateStoreSell: need %d bytes, got %d: %w", privateStoreDealHeader, r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestPrivateStoreSell{StoreID: r.ReadInt32()}
	count := r.ReadInt32()
	if !storeRowsFit(count, 1, maxItems, storeSaleRowSize, r.Remaining()) {
		return req, nil
	}
	items := make([]StoreSaleRow, count)
	valid := true
	for i := range items {
		row := StoreSaleRow{ObjectID: r.ReadInt32(), ItemID: r.ReadInt32(), Enchant: int(r.ReadUint16())}
		r.ReadUint16()
		row.Count = r.ReadInt32()
		row.Price = r.ReadInt32()
		if row.ObjectID < 1 || row.ItemID < 1 || row.Count < 1 || row.Price < 0 {
			valid = false
			break
		}
		items[i] = row
	}
	if err := r.Err(); err != nil {
		return RequestPrivateStoreSell{}, fmt.Errorf("clientpackets: RequestPrivateStoreSell: %w", err)
	}
	if valid {
		req.Items = items
	}
	return req, nil
}

// StoreMessage carries a store title or workshop name
// (SetPrivateStoreMsgSell, SetPrivateStoreMsgBuy, RequestRecipeShopMessageSet).
type StoreMessage struct {
	Text string
}

// DecodeStoreMessage parses a raw store title or workshop name payload
// (opcode byte included).
func DecodeStoreMessage(payload []byte) (StoreMessage, error) {
	r := newReader(payload)
	req := StoreMessage{Text: r.ReadString()}
	if err := r.Err(); err != nil {
		return StoreMessage{}, fmt.Errorf("clientpackets: store message: %w", err)
	}
	return req, nil
}

// RecipeShopRow is one row of RequestRecipeShopListSet.
type RecipeShopRow struct {
	RecipeID int32
	Cost     int32
}

// RequestRecipeShopListSet sets the workshop's recipes. A row count out of
// range or disagreeing with the packet's length reads as no rows.
type RequestRecipeShopListSet struct {
	Items []RecipeShopRow
}

// DecodeRequestRecipeShopListSet parses a raw RequestRecipeShopListSet
// payload (opcode byte included) of at most maxItems rows.
func DecodeRequestRecipeShopListSet(payload []byte, maxItems int) (RequestRecipeShopListSet, error) {
	r := newReader(payload)
	if r.Remaining() < privateStoreHeaderSize {
		return RequestRecipeShopListSet{}, fmt.Errorf("clientpackets: RequestRecipeShopListSet: need %d bytes, got %d: %w", privateStoreHeaderSize, r.Remaining(), wire.ErrShortPacket)
	}
	count := r.ReadInt32()
	var req RequestRecipeShopListSet
	if !storeRowsFit(count, 0, maxItems, recipeShopRowSize, r.Remaining()) {
		return req, nil
	}
	req.Items = make([]RecipeShopRow, count)
	for i := range req.Items {
		req.Items[i] = RecipeShopRow{RecipeID: r.ReadInt32(), Cost: r.ReadInt32()}
	}
	if err := r.Err(); err != nil {
		return RequestRecipeShopListSet{}, fmt.Errorf("clientpackets: RequestRecipeShopListSet: %w", err)
	}
	return req, nil
}

// RequestRecipeShopMakeInfo asks a workshop's craft window for RecipeID.
type RequestRecipeShopMakeInfo struct {
	CrafterID int32
	RecipeID  int32
}

// DecodeRequestRecipeShopMakeInfo parses a raw RequestRecipeShopMakeInfo
// payload (opcode byte included).
func DecodeRequestRecipeShopMakeInfo(payload []byte) (RequestRecipeShopMakeInfo, error) {
	r := newReader(payload)
	if r.Remaining() < recipeShopMakeInfoSize {
		return RequestRecipeShopMakeInfo{}, fmt.Errorf("clientpackets: RequestRecipeShopMakeInfo: need %d bytes, got %d: %w", recipeShopMakeInfoSize, r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestRecipeShopMakeInfo{CrafterID: r.ReadInt32(), RecipeID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestRecipeShopMakeInfo{}, fmt.Errorf("clientpackets: RequestRecipeShopMakeInfo: %w", err)
	}
	return req, nil
}

// RequestRecipeShopMakeItem orders a craft of RecipeID from a workshop.
type RequestRecipeShopMakeItem struct {
	CrafterID int32
	RecipeID  int32
}

// DecodeRequestRecipeShopMakeItem parses a raw RequestRecipeShopMakeItem
// payload (opcode byte included). Its trailing field is read and unused.
func DecodeRequestRecipeShopMakeItem(payload []byte) (RequestRecipeShopMakeItem, error) {
	r := newReader(payload)
	if r.Remaining() < recipeShopMakeItemSize {
		return RequestRecipeShopMakeItem{}, fmt.Errorf("clientpackets: RequestRecipeShopMakeItem: need %d bytes, got %d: %w", recipeShopMakeItemSize, r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestRecipeShopMakeItem{CrafterID: r.ReadInt32(), RecipeID: r.ReadInt32()}
	r.ReadInt32()
	if err := r.Err(); err != nil {
		return RequestRecipeShopMakeItem{}, fmt.Errorf("clientpackets: RequestRecipeShopMakeItem: %w", err)
	}
	return req, nil
}

// storeRowsFit reports whether a store packet's row count lies within
// [minCount, maxItems] and its rows fill exactly the remaining bytes.
func storeRowsFit(count int32, minCount, maxItems, rowSize, remaining int) bool {
	return count >= int32(minCount) && int(count) <= maxItems && int(count)*rowSize == remaining
}
