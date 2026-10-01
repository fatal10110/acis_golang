package clientpackets

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ---- from warehouse_test.go ----
func TestDecodeWarehouseItemBatchPackets(t *testing.T) {
	payload := []byte{
		OpcodeSendWarehouseDeposit,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	deposit, err := DecodeSendWarehouseDepositList(payload, defaultMaxItemInPacket)
	if err != nil {
		t.Fatalf("DecodeSendWarehouseDepositList: %v", err)
	}
	withdraw, err := DecodeSendWarehouseWithdrawList(append([]byte{OpcodeSendWarehouseWithdraw}, payload[1:]...), defaultMaxItemInPacket)
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
			if _, err := DecodeSendWarehouseDepositList(tt.payload, defaultMaxItemInPacket); err == nil {
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

	got, err := DecodeRequestPackageSend(payload, defaultMaxItemInPacket)
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

	got, err := DecodeRequestPackageSend(payload, defaultMaxItemInPacket)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSend: %v", err)
	}
	if got.ObjectID != 0x12345678 || len(got.Items) != 0 {
		t.Fatalf("DecodeRequestPackageSend = %+v, want object id with no items", got)
	}
}

// TestDecodeRequestPackageSendCountAboveMaxIsNotShortPacket proves a
// count exceeding the item-list cap (mirroring Config.MAX_ITEM_IN_PACKET,
// whose readImpl() guard returns silently before any row read, so it can
// never throw BufferUnderflowException) is a plain validation error, not
// classified as a buffer-underflow-equivalent wire.ErrShortPacket -- even
// though its trailing byte count is also short for that count, matching
// the reference's guard order (count bound checked first, silently).
func TestDecodeRequestPackageSendCountAboveMaxIsNotShortPacket(t *testing.T) {
	payload := []byte{
		OpcodeRequestPackageSend,
		0x78, 0x56, 0x34, 0x12,
		0x65, 0x00, 0x00, 0x00, // count = 101, exceeds the default cap (100)
	}

	_, err := DecodeRequestPackageSend(payload, defaultMaxItemInPacket)
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
			if _, err := DecodeRequestPackageSend(tt.payload, defaultMaxItemInPacket); err == nil {
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

// TestDecodeRecipeBookRequests pins the four recipe book requests: one
// little-endian int32 each after the opcode. RequestRecipeBookOpen reads a
// type of 0 as the dwarven page and anything else as the common one.
func TestDecodeRecipeBookRequests(t *testing.T) {
	id := []byte{0xae, 0x02, 0x00, 0x00} // 686
	if got, err := DecodeRequestRecipeBookDestroy(append([]byte{0xad}, id...)); err != nil || got.RecipeID != 686 {
		t.Fatalf("DecodeRequestRecipeBookDestroy = %+v, %v", got, err)
	}
	if got, err := DecodeRequestRecipeItemMakeInfo(append([]byte{0xae}, id...)); err != nil || got.RecipeID != 686 {
		t.Fatalf("DecodeRequestRecipeItemMakeInfo = %+v, %v", got, err)
	}
	if got, err := DecodeRequestRecipeItemMakeSelf(append([]byte{0xaf}, id...)); err != nil || got.RecipeID != 686 {
		t.Fatalf("DecodeRequestRecipeItemMakeSelf = %+v, %v", got, err)
	}
	for _, tc := range []struct {
		typ  byte
		want bool
	}{{0, true}, {1, false}, {7, false}} {
		got, err := DecodeRequestRecipeBookOpen([]byte{0xac, tc.typ, 0, 0, 0})
		if err != nil || got.Dwarven != tc.want {
			t.Fatalf("DecodeRequestRecipeBookOpen(type %d) = %+v, %v; want dwarven %v", tc.typ, got, err, tc.want)
		}
	}
	if _, err := DecodeRequestRecipeItemMakeSelf([]byte{0xaf, 1, 0}); err == nil {
		t.Fatal("DecodeRequestRecipeItemMakeSelf: want error on short payload")
	}
}

// TestDecodeRequestPreviewItem pins the try-on request layout: an unused
// int, the list id, the item count, then one item id per item. A negative
// count reads as none, a count over 100 is refused without reading the
// rows, and missing rows are a short packet.
func TestDecodeRequestPreviewItem(t *testing.T) {
	payload, _ := hex.DecodeString("c6" + "07000000" + "01000000" + "02000000" + "41090000" + "50040000")
	got, err := DecodeRequestPreviewItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPreviewItem: %v", err)
	}
	if got.ListID != 1 || len(got.Items) != 2 || got.Items[0] != 2369 || got.Items[1] != 1104 {
		t.Fatalf("DecodeRequestPreviewItem = %+v, want list 1 items [2369 1104]", got)
	}

	negative, _ := hex.DecodeString("c6" + "00000000" + "05000000" + "ffffffff")
	if got, err := DecodeRequestPreviewItem(negative); err != nil || got.ListID != 5 || len(got.Items) != 0 {
		t.Fatalf("negative count = %+v, %v; want list 5 and no items", got, err)
	}
	tooMany, _ := hex.DecodeString("c6" + "00000000" + "05000000" + "65000000")
	if _, err := DecodeRequestPreviewItem(tooMany); err == nil || errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("101 items: err = %v, want a refusal that is not a short packet", err)
	}
	short, _ := hex.DecodeString("c6" + "00000000" + "05000000" + "02000000" + "41090000")
	if _, err := DecodeRequestPreviewItem(short); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("missing row: err = %v, want a short packet", err)
	}
}

// TestDecodeSymbolMakerRequests pins the six symbol maker requests: one
// little-endian int32 each after the opcode, a symbol id for the four that
// name one; a payload short of it is an error.
func TestDecodeSymbolMakerRequests(t *testing.T) {
	id := []byte{0x07, 0x00, 0x00, 0x00}
	for _, tc := range []struct {
		name   string
		decode func([]byte) (RequestHennaSymbol, error)
		opcode byte
	}{
		{"RequestHennaItemInfo", DecodeRequestHennaItemInfo, OpcodeRequestHennaItemInfo},
		{"RequestHennaEquip", DecodeRequestHennaEquip, OpcodeRequestHennaEquip},
		{"RequestHennaUnequipInfo", DecodeRequestHennaUnequipInfo, OpcodeRequestHennaUnequipInfo},
		{"RequestHennaUnequip", DecodeRequestHennaUnequip, OpcodeRequestHennaUnequip},
	} {
		if got, err := tc.decode(append([]byte{tc.opcode}, id...)); err != nil || got.SymbolID != 7 {
			t.Fatalf("%s = %+v, %v; want symbol 7", tc.name, got, err)
		}
		if _, err := tc.decode([]byte{tc.opcode, 7, 0}); err == nil {
			t.Fatalf("%s: want error on short payload", tc.name)
		}
	}
	if _, err := DecodeRequestHennaItemList(append([]byte{OpcodeRequestHennaItemList}, id...)); err != nil {
		t.Fatalf("DecodeRequestHennaItemList: %v", err)
	}
	if _, err := DecodeRequestHennaUnequipList(append([]byte{OpcodeRequestHennaUnequipList}, id...)); err != nil {
		t.Fatalf("DecodeRequestHennaUnequipList: %v", err)
	}
	if _, err := DecodeRequestHennaItemList([]byte{OpcodeRequestHennaItemList}); err == nil {
		t.Fatal("DecodeRequestHennaItemList: want error on short payload")
	}
	if _, err := DecodeRequestHennaUnequipList([]byte{OpcodeRequestHennaUnequipList}); err == nil {
		t.Fatal("DecodeRequestHennaUnequipList: want error on short payload")
	}
}

// storeRows builds a store request: the opcode, the header ints, then the
// raw row bytes.
func storeRows(opcode byte, header []int32, rows ...[]byte) []byte {
	out := []byte{opcode}
	for _, v := range header {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	for _, row := range rows {
		out = append(out, row...)
	}
	return out
}

func le32(vs ...int32) []byte {
	var out []byte
	for _, v := range vs {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// TestDecodeSetPrivateStoreLists pins the two list set-ups:
// SetPrivateStoreListSell (package flag D, count D, then object, count,
// price per row) and SetPrivateStoreListBuy (count D, then item D, enchant
// H, an unread H, count D, price D per row). A count below 1, above the
// cap, or disagreeing with the rows' length decodes as no rows, not an
// error.
func TestDecodeSetPrivateStoreLists(t *testing.T) {
	sell := storeRows(0x74, []int32{1, 2}, le32(500, 3, 100), le32(501, 1, 9000))
	got, err := DecodeSetPrivateStoreListSell(sell, 10)
	if err != nil || !got.Packaged || len(got.Items) != 2 || got.Items[1] != (StoreSellRow{ObjectID: 501, Count: 1, Price: 9000}) {
		t.Fatalf("DecodeSetPrivateStoreListSell = %+v, %v", got, err)
	}
	if got, err := DecodeSetPrivateStoreListSell(storeRows(0x74, []int32{2, 1}, le32(500, 3, 100)), 10); err != nil || got.Packaged || len(got.Items) != 1 {
		t.Fatalf("package flag 2 = %+v, %v; want a plain sale", got, err)
	}
	for name, payload := range map[string][]byte{
		"zero rows":       storeRows(0x74, []int32{0, 0}),
		"over the cap":    storeRows(0x74, []int32{0, 2}, le32(500, 3, 100), le32(501, 1, 9000)),
		"short of a row":  storeRows(0x74, []int32{0, 2}, le32(500, 3, 100)),
		"negative counts": storeRows(0x74, []int32{0, -1}),
	} {
		max := 10
		if name == "over the cap" {
			max = 1
		}
		if got, err := DecodeSetPrivateStoreListSell(payload, max); err != nil || got.Items != nil {
			t.Fatalf("%s: DecodeSetPrivateStoreListSell = %+v, %v; want no rows", name, got, err)
		}
	}
	if _, err := DecodeSetPrivateStoreListSell([]byte{0x74, 1, 0, 0, 0}, 10); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short SetPrivateStoreListSell err = %v, want ErrShortPacket", err)
	}

	row := append(le32(1060), 0x05, 0x00, 0xff, 0xff)
	row = append(row, le32(20, 35)...)
	buy, err := DecodeSetPrivateStoreListBuy(storeRows(0x91, []int32{1}, row), 10)
	if err != nil || len(buy.Items) != 1 || buy.Items[0] != (StoreBuyRow{ItemID: 1060, Enchant: 5, Count: 20, Price: 35}) {
		t.Fatalf("DecodeSetPrivateStoreListBuy = %+v, %v", buy, err)
	}
	if buy, err := DecodeSetPrivateStoreListBuy(storeRows(0x91, []int32{2}, row), 10); err != nil || buy.Items != nil {
		t.Fatalf("SetPrivateStoreListBuy short of a row = %+v, %v; want no rows", buy, err)
	}
}

// TestDecodePrivateStoreDeals pins RequestPrivateStoreBuy (store D, count D,
// then object, count, price per row) and RequestPrivateStoreSell (store D,
// count D, then object D, item D, enchant H, an unread H, count D, price D
// per row). A bad row count, or a row naming an object, item or count below
// 1 or a negative price, decodes as no rows, not an error.
func TestDecodePrivateStoreDeals(t *testing.T) {
	buy, err := DecodeRequestPrivateStoreBuy(storeRows(0x79, []int32{100, 1}, le32(500, 2, 40)), 10)
	if err != nil || buy.StoreID != 100 || len(buy.Items) != 1 || buy.Items[0] != (StorePurchaseRow{ObjectID: 500, Count: 2, Price: 40}) {
		t.Fatalf("DecodeRequestPrivateStoreBuy = %+v, %v", buy, err)
	}
	for name, row := range map[string][]byte{
		"object 0":       le32(0, 2, 40),
		"count 0":        le32(500, 0, 40),
		"negative price": le32(500, 2, -1),
	} {
		if got, err := DecodeRequestPrivateStoreBuy(storeRows(0x79, []int32{100, 1}, row), 10); err != nil || got.Items != nil {
			t.Fatalf("%s: DecodeRequestPrivateStoreBuy = %+v, %v; want no rows", name, got, err)
		}
	}

	sale := func(objectID, itemID int32, enchant uint16, count, price int32) []byte {
		out := le32(objectID, itemID)
		out = binary.LittleEndian.AppendUint16(out, enchant)
		out = append(out, 0xff, 0xff)
		return append(out, le32(count, price)...)
	}
	sell, err := DecodeRequestPrivateStoreSell(storeRows(0x96, []int32{100, 1}, sale(600, 1, 65535, 1, 9000)), 10)
	if err != nil || sell.StoreID != 100 || len(sell.Items) != 1 || sell.Items[0] != (StoreSaleRow{ObjectID: 600, ItemID: 1, Enchant: 65535, Count: 1, Price: 9000}) {
		t.Fatalf("DecodeRequestPrivateStoreSell = %+v, %v", sell, err)
	}
	if got, err := DecodeRequestPrivateStoreSell(storeRows(0x96, []int32{100, 1}, sale(600, 0, 0, 1, 9000)), 10); err != nil || got.Items != nil {
		t.Fatalf("item 0: DecodeRequestPrivateStoreSell = %+v, %v; want no rows", got, err)
	}
	if _, err := DecodeRequestPrivateStoreSell([]byte{0x96, 1, 0, 0, 0}, 10); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short RequestPrivateStoreSell err = %v, want ErrShortPacket", err)
	}
}

// TestDecodeRecipeShopRequests pins the workshop requests: the list set-up
// (count D then recipe, cost per row, a bad count reading as no rows), the
// craft window (crafter, recipe) and the order (crafter, recipe, an unread
// int), and the store title (one null-terminated UTF-16 string).
func TestDecodeRecipeShopRequests(t *testing.T) {
	list, err := DecodeRequestRecipeShopListSet(storeRows(0xb2, []int32{2}, le32(1, 300), le32(686, 0)), 20)
	if err != nil || len(list.Items) != 2 || list.Items[1] != (RecipeShopRow{RecipeID: 686}) {
		t.Fatalf("DecodeRequestRecipeShopListSet = %+v, %v", list, err)
	}
	if list, err := DecodeRequestRecipeShopListSet(storeRows(0xb2, []int32{3}, le32(1, 300)), 20); err != nil || len(list.Items) != 0 {
		t.Fatalf("bad count: DecodeRequestRecipeShopListSet = %+v, %v; want no rows", list, err)
	}
	info, err := DecodeRequestRecipeShopMakeInfo(storeRows(0xb5, []int32{100, 686}))
	if err != nil || info != (RequestRecipeShopMakeInfo{CrafterID: 100, RecipeID: 686}) {
		t.Fatalf("DecodeRequestRecipeShopMakeInfo = %+v, %v", info, err)
	}
	order, err := DecodeRequestRecipeShopMakeItem(storeRows(0xb6, []int32{100, 686, 7}))
	if err != nil || order != (RequestRecipeShopMakeItem{CrafterID: 100, RecipeID: 686}) {
		t.Fatalf("DecodeRequestRecipeShopMakeItem = %+v, %v", order, err)
	}
	if _, err := DecodeRequestRecipeShopMakeItem(storeRows(0xb6, []int32{100, 686})); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("short RequestRecipeShopMakeItem err = %v, want ErrShortPacket", err)
	}
	msg, err := DecodeStoreMessage([]byte{0x77, 'h', 0, 'i', 0, 0, 0})
	if err != nil || msg.Text != "hi" {
		t.Fatalf("DecodeStoreMessage = %+v, %v", msg, err)
	}
	if _, err := DecodeStoreMessage([]byte{0x77, 'h', 0}); !errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("unterminated store message err = %v, want ErrShortPacket", err)
	}
}

// TestDecodeMultiSellChoose pins MultiSellChoose (0xa7): three
// little-endian int32 after the opcode — list id, entry id, amount — the
// reference's readD() order. A payload short of the three is an error.
func TestDecodeMultiSellChoose(t *testing.T) {
	payload := []byte{
		0xa7,
		0xea, 0x03, 0x00, 0x00, // list 1002
		0x02, 0x00, 0x00, 0x00, // entry 2
		0x0f, 0x27, 0x00, 0x00, // amount 9999
	}
	got, err := DecodeMultiSellChoose(payload)
	if err != nil || got != (MultiSellChoose{ListID: 1002, EntryID: 2, Amount: 9999}) {
		t.Fatalf("DecodeMultiSellChoose = %+v, %v", got, err)
	}
	if _, err := DecodeMultiSellChoose(payload[:12]); err == nil {
		t.Fatal("DecodeMultiSellChoose: want error on short payload")
	}
}
