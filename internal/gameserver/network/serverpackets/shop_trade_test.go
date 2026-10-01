package serverpackets

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
)

// ---- from exsendmanorlist_test.go ----
func TestFrameExSendManorList(t *testing.T) {
	got := framePayload(t, FrameExSendManorList())

	var want []byte
	want = append(want, OpcodeExtended)
	want = binary.LittleEndian.AppendUint16(want, OpcodeExSendManorList)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(manorNames)))
	for i, name := range manorNames {
		want = binary.LittleEndian.AppendUint32(want, uint32(i+1))
		var w wire.Writer
		w.WriteString(name)
		want = append(want, w.Bytes()...)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameExSendManorList() = % x, want % x", got, want)
	}
}

// ---- from shop_trade_test.go ----

// TestFrameBuyList pins the BuyList rows: each limited product shows its
// current count and one sold out is left out while the header still counts
// it (BuyList.java:30-34), and an unlimited product shows count 0.
func TestFrameBuyList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
		{ID: 2369, Kind: item.KindWeapon, Slot: item.SlotRHand},
	})
	list := buylist.List{ID: 101, Products: []buylist.Product{
		{ItemID: 57, Price: 1, MaxCount: -1},
		{ItemID: 2368, Price: 625, MaxCount: 3},
		{ItemID: 2369, Price: 900, MaxCount: 5},
	}}
	stock := map[int32]int{2368: 2, 2369: 0}
	count := func(p buylist.Product) int { return stock[p.ItemID] }

	frame, err := FrameBuyList(list, count, 123456, 0.10, 1.0, templates)
	if err != nil {
		t.Fatalf("FrameBuyList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeBuyList}
	want = binary.LittleEndian.AppendUint32(want, 123456)
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 57, 57, 0, item.SubCategoryMoney, item.SlotNone, 0, 0, 0, 1)
	want = appendShopTradeItem(want, item.CategoryWeaponOrJewelry, 2368, 2368, 2, item.SubCategoryWeapon, item.SlotLRHand, 0, 0, 0, 687)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameBuyList() = %x, want %x", got, want)
	}
}

func noStock(buylist.Product) int { return 0 }

// TestFrameBuyListSiegeGuardPrice proves siege-guard buylist items (IDs
// 3960-4026) price with Config.RATE_SIEGE_GUARDS_PRICE in addition to tax,
// per BuyList.java:47-50, while items outside that range use the plain
// price*(1+taxRate) formula.
func TestFrameBuyListSiegeGuardPrice(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 3960, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 4026, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 4027, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
	list := buylist.List{ID: 1, Products: []buylist.Product{
		{ItemID: 3960, Price: 1000, MaxCount: -1},
		{ItemID: 4026, Price: 1000, MaxCount: -1},
		{ItemID: 4027, Price: 1000, MaxCount: -1},
	}}

	frame, err := FrameBuyList(list, noStock, 0, 0.10, 2.0, templates)
	if err != nil {
		t.Fatalf("FrameBuyList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeBuyList}
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	// Siege-guard range: price * rate(2.0) * (1+tax) = 1000*2.0*1.10 = 2200.
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 3960, 3960, 0, item.SubCategoryOther, item.SlotNone, 0, 0, 0, 2200)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 4026, 4026, 0, item.SubCategoryOther, item.SlotNone, 0, 0, 0, 2200)
	// Outside range: price * (1+tax) = 1000*1.10 = 1100, rate not applied.
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 4027, 4027, 0, item.SubCategoryOther, item.SlotNone, 0, 0, 0, 1100)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameBuyList() = %x, want %x", got, want)
	}
}

func TestFrameSellList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone, ReferencePrice: 1},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, ReferencePrice: 2500},
	})
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 57, Count: 1000, Location: item.LocationInventory},
		{ObjectID: 501, TemplateID: 1146, Count: 1, EnchantLevel: 3, CustomType1: 4, CustomType2: 5, Location: item.LocationPaperdoll},
	}

	frame, err := FrameSellList(3000, items, templates)
	if err != nil {
		t.Fatalf("FrameSellList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeSellList}
	want = binary.LittleEndian.AppendUint32(want, 3000)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 1000, item.SubCategoryMoney, item.SlotNone, 0, 0, 0, 0)
	want = appendShopTradeItem(want, item.CategoryArmor, 501, 1146, 1, item.SubCategoryArmor, item.SlotChest, 3, 4, 5, 1250)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSellList() = %x, want %x", got, want)
	}
}

func TestFrameTradePackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"request", framePayload(t, FrameSendTradeRequest(42)), []byte{OpcodeSendTradeRequest, 42, 0, 0, 0}},
		{"done success", framePayload(t, FrameSendTradeDone(true)), []byte{OpcodeSendTradeDone, 1, 0, 0, 0}},
		{"done failure", framePayload(t, FrameSendTradeDone(false)), []byte{OpcodeSendTradeDone, 0, 0, 0, 0}},
		{"press own", framePayload(t, FrameTradePressOwnOk()), []byte{OpcodeTradePressOwnOk}},
		{"press other", framePayload(t, FrameTradePressOtherOk()), []byte{OpcodeTradePressOtherOk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("%s = %x, want %x", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestFrameTradeStartAndAdd(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone, Tradable: true},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Tradable: true},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, Tradable: true},
	})
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 57, Count: 1000, Location: item.LocationInventory},
		{ObjectID: 501, TemplateID: 2368, Count: 1, EnchantLevel: 3, Location: item.LocationInventory},
		{ObjectID: 502, TemplateID: 2368, Count: 1, Location: item.LocationWarehouse},
		{ObjectID: 503, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll},
	}

	frame, err := FrameTradeStart(42, items, templates)
	if err != nil {
		t.Fatalf("FrameTradeStart: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeTradeStart}
	want = binary.LittleEndian.AppendUint32(want, 42)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 1000, item.SubCategoryMoney, item.SlotNone, 0)
	want = appendTradeItem(want, item.CategoryWeaponOrJewelry, 501, 2368, 1, item.SubCategoryWeapon, item.SlotLRHand, 3)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTradeStart() = %x, want %x", got, want)
	}

	add := TradeItemSnapshot{ObjectID: 501, TemplateID: 2368, Count: 1, EnchantLevel: 3}
	own, err := FrameTradeOwnAdd(add, 1, templates)
	if err != nil {
		t.Fatalf("FrameTradeOwnAdd: %v", err)
	}
	other, err := FrameTradeOtherAdd(add, 1, templates)
	if err != nil {
		t.Fatalf("FrameTradeOtherAdd: %v", err)
	}
	addPayload := append([]byte{OpcodeTradeOwnAdd}, binary.LittleEndian.AppendUint16(nil, 1)...)
	addPayload = appendTradeItem(addPayload, item.CategoryWeaponOrJewelry, 501, 2368, 1, item.SubCategoryWeapon, item.SlotLRHand, 3)
	if got := framePayload(t, own); !bytes.Equal(got, addPayload) {
		t.Fatalf("FrameTradeOwnAdd() = %x, want %x", got, addPayload)
	}
	addPayload[0] = OpcodeTradeOtherAdd
	if got := framePayload(t, other); !bytes.Equal(got, addPayload) {
		t.Fatalf("FrameTradeOtherAdd() = %x, want %x", got, addPayload)
	}
}

func TestFrameTradeUpdatePackets(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone, Stackable: true},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
	})
	stack := TradeItemSnapshot{ObjectID: 500, TemplateID: 57, Count: 10}
	weapon := TradeItemSnapshot{ObjectID: 501, TemplateID: 2368, Count: 1, EnchantLevel: 3}

	frame, err := FrameTradeUpdate(stack, 90, templates)
	if err != nil {
		t.Fatalf("FrameTradeUpdate: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{OpcodeTradeUpdate}
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = appendTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 90, item.SubCategoryMoney, item.SlotNone, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTradeUpdate(stack) = %x, want %x", got, want)
	}

	frame, err = FrameTradeItemUpdate([]TradeItemUpdateEntry{
		{Item: stack, AvailableCount: 90},
		{Item: weapon, AvailableCount: 0},
	}, templates)
	if err != nil {
		t.Fatalf("FrameTradeItemUpdate: %v", err)
	}
	got = framePayload(t, frame)
	want = []byte{OpcodeTradeItemUpdate}
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = appendTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 90, item.SubCategoryMoney, item.SlotNone, 0)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendTradeItem(want, item.CategoryWeaponOrJewelry, 501, 2368, 1, item.SubCategoryWeapon, item.SlotLRHand, 3)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTradeItemUpdate() = %x, want %x", got, want)
	}
}

func TestFrameShopTradeMissingTemplate(t *testing.T) {
	if _, err := FrameBuyList(buylist.List{ID: 1, Products: []buylist.Product{{ItemID: 9, MaxCount: -1}}}, noStock, 0, 0, 1.0, item.NewTable(nil)); err == nil {
		t.Fatal("FrameBuyList: want missing-template error")
	}
	if _, err := FrameSellList(0, []*item.Instance{{TemplateID: 9}}, item.NewTable(nil)); err == nil {
		t.Fatal("FrameSellList: want missing-template error")
	}
	if _, err := FrameTradeStart(1, []*item.Instance{{TemplateID: 9, Location: item.LocationInventory}}, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeStart: want missing-template error")
	}
	if _, err := FrameTradeOwnAdd(TradeItemSnapshot{TemplateID: 9}, 1, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeOwnAdd: want missing-template error")
	}
	if _, err := FrameTradeUpdate(TradeItemSnapshot{TemplateID: 9}, 1, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeUpdate: want missing-template error")
	}
	if _, err := FrameTradeItemUpdate([]TradeItemUpdateEntry{{Item: TradeItemSnapshot{TemplateID: 9}}}, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeItemUpdate: want missing-template error")
	}
}

func appendShopTradeItem(dst []byte, category item.Category, objectID, templateID, count int32, subCategory item.SubCategory, slot item.Slot, enchant, custom1, custom2 int, price int32) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, uint16(category))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(objectID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(templateID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(count))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(subCategory))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(custom1))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(slot))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(enchant))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(custom2))
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(price))
	return dst
}

func appendTradeItem(dst []byte, category item.Category, objectID, templateID, count int32, subCategory item.SubCategory, slot item.Slot, enchant int) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, uint16(category))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(objectID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(templateID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(count))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(subCategory))
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(slot))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(enchant))
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	return dst
}

// TestFrameShopPreviewList pins the try-on window bytes: four fixed bytes
// after the opcode, the adena, the list id, then the equipable products of
// a grade within the expertise level, each with its type2, a 16-bit slot
// and the wear price (ShopPreviewList.java).
func TestFrameShopPreviewList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1101, Kind: item.KindArmor, Slot: item.SlotChest, Crystal: item.CrystalNone},
		{ID: 2369, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD},
		{ID: 160, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalC},
		{ID: 1060, Kind: item.KindEtcItem, Slot: item.SlotNone, Stackable: true},
		{ID: 8177, Kind: item.KindArmor, Slot: item.SlotHairAll, Crystal: item.CrystalNone},
	})
	list := buylist.List{ID: 7, Products: []buylist.Product{{ItemID: 1101}, {ItemID: 2369}, {ItemID: 160}, {ItemID: 1060}, {ItemID: 8177}}}
	frame, err := FrameShopPreviewList(list, 5000, 1, 10, templates)
	if err != nil {
		t.Fatalf("FrameShopPreviewList: %v", err)
	}
	want, _ := hex.DecodeString("ef" + "c0130000" + "88130000" + "07000000" + "0300" +
		"4d040000" + "0100" + "0004" + "0a000000" +
		"41090000" + "0000" + "8000" + "0a000000" +
		"f11f0000" + "0200" + "0000" + "0a000000")
	if got := framePayload(t, frame); !bytes.Equal(got, want) {
		t.Fatalf("FrameShopPreviewList = %x, want %x", got, want)
	}
}

// TestFrameShopPreviewInfo pins the tried-on items packet: the slot count
// 17, then the item id at each paperdoll position in the order REAR, LEAR,
// NECK, RFINGER, LFINGER, HEAD, RHAND, LHAND, GLOVES, CHEST, LEGS, FEET,
// CLOAK, FACE, HAIR, HAIRALL, UNDER (ShopPreviewInfo.java).
func TestFrameShopPreviewInfo(t *testing.T) {
	var items [item.PaperdollSlots]int32
	items[0], items[1], items[2], items[7], items[10] = 100, 101, 102, 2369, 1101
	want, _ := hex.DecodeString("f0" + "11000000" +
		"66000000" + "65000000" + "00000000" + "00000000" + "00000000" + "00000000" +
		"41090000" + "00000000" + "00000000" + "4d040000" + "00000000" + "00000000" +
		"00000000" + "00000000" + "00000000" + "00000000" + "64000000")
	if got := framePayload(t, FrameShopPreviewInfo(items)); !bytes.Equal(got, want) {
		t.Fatalf("FrameShopPreviewInfo = %x, want %x", got, want)
	}
}

// TestFrameSymbolMakerPackets pins the four symbol maker windows:
// HennaEquipList (0xe2: adena, max slots, count, then symbol, dye, 10,
// price, 1 per symbol), HennaUnequipList (0xe5: adena, empty slots, count,
// then symbol, dye, 5, price/5, 1), and HennaItemInfo (0xe3) and
// HennaItemUnequipInfo (0xe6): symbol, dye, dye count, price, 1, adena, then
// per attribute in INT, STR, CON, MEN, DEX, WIT order the current value as
// an int32 and the changed value's low byte, so a value below zero wraps.
func TestFrameSymbolMakerPackets(t *testing.T) {
	h := henna.Henna{SymbolID: 1, DyeID: 4445, DrawPrice: 37000, STR: 1, CON: -3}
	other := henna.Henna{SymbolID: 7, DyeID: 4451, DrawPrice: 37000, INT: 1, MEN: -3}

	got := framePayload(t, FrameHennaEquipList(148000, 2, []henna.Henna{h, other}))
	want := []byte{
		0xe2,
		0x20, 0x42, 0x02, 0x00, // 148000 adena
		0x02, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x5d, 0x11, 0x00, 0x00, 0x0a, 0x00, 0x00, 0x00, 0x88, 0x90, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x07, 0x00, 0x00, 0x00, 0x63, 0x11, 0x00, 0x00, 0x0a, 0x00, 0x00, 0x00, 0x88, 0x90, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaEquipList() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameHennaUnequipList(5, 1, []henna.Henna{h}))
	want = []byte{
		0xe5,
		0x05, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x5d, 0x11, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0xe8, 0x1c, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaUnequipList() = %x, want %x", got, want)
	}

	now := HennaStats{INT: 21, STR: 40, CON: 2, MEN: 25, DEX: 30, WIT: 11}
	head := []byte{
		0x01, 0x00, 0x00, 0x00, 0x5d, 0x11, 0x00, 0x00,
	}
	got = framePayload(t, FrameHennaItemInfo(h, 9, now))
	want = append([]byte{0xe3}, head...)
	want = append(want,
		0x0a, 0x00, 0x00, 0x00, 0x88, 0x90, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00,
		21, 0, 0, 0, 21,
		40, 0, 0, 0, 41,
		2, 0, 0, 0, 0xff, // CON 2 - 3 wraps to 255
		25, 0, 0, 0, 25,
		30, 0, 0, 0, 30,
		11, 0, 0, 0, 11,
	)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaItemInfo() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameHennaItemUnequipInfo(h, 9, now))
	want = append([]byte{0xe6}, head...)
	want = append(want,
		0x05, 0x00, 0x00, 0x00, 0xe8, 0x1c, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00,
		21, 0, 0, 0, 21,
		40, 0, 0, 0, 39,
		2, 0, 0, 0, 5,
		25, 0, 0, 0, 25,
		30, 0, 0, 0, 30,
		11, 0, 0, 0, 11,
	)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaItemUnequipInfo() = %x, want %x", got, want)
	}
}

// ---- multisell ----

func multisellIngredient(t *testing.T, items *item.Table, attrs ...string) multisell.Ingredient {
	t.Helper()
	set := commons.NewStatSet()
	for i := 0; i+1 < len(attrs); i += 2 {
		set.Set(attrs[i], attrs[i+1])
	}
	in, err := multisell.NewIngredient(set, items)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// TestFrameMultiSellList pins MultiSellList (0xd0) against the reference
// writeImpl: list id, 1-based page, finished flag, page size 40, entry
// count; per entry its 1-based number across the list, two zero ints, the
// stackable byte, product and ingredient counts (2 bytes each); per product
// item id (2 bytes), body part, type2 (2 bytes), count, enchant (2 bytes)
// and two zero ints; per ingredient the same without the body part. An
// item without a template writes body part 0 and type2 65535. A prepared
// entry drops its tax adena and merges its adena at the end.
func TestFrameMultiSellList(t *testing.T) {
	items := item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 1000, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{}},
	})
	sword := multisellIngredient(t, items, "id", "1000", "count", "1")
	unknown := multisellIngredient(t, items, "id", "4242", "count", "3")
	entries := []multisell.Entry{multisell.NewEntry(
		[]multisell.Ingredient{
			multisellIngredient(t, items, "id", "57", "count", "500"),
			multisellIngredient(t, items, "id", "57", "count", "9000", "isTaxIngredient", "true"),
			unknown,
			multisellIngredient(t, items, "id", "57", "count", "20"),
		},
		[]multisell.Ingredient{sword},
	)}
	for range 40 {
		entries = append(entries, multisell.NewEntry([]multisell.Ingredient{sword}, []multisell.Ingredient{unknown}))
	}
	list := (&multisell.List{ID: -7, Entries: entries}).Prepare()

	got := framePayload(t, FrameMultiSellList(list, 0))
	want := []byte{OpcodeMultiSellList}
	for _, v := range []int32{-7, 1, 0, 40, 40} {
		want = binary.LittleEndian.AppendUint32(want, uint32(v))
	}
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = append(want, 0)                              // a weapon product does not stack
	want = binary.LittleEndian.AppendUint16(want, 1)    // products
	want = binary.LittleEndian.AppendUint16(want, 2)    // ingredients: the unknown item, then adena 520
	want = binary.LittleEndian.AppendUint16(want, 1000) // product: sword
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotRHand))
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = append(want, make([]byte, 8)...)
	want = binary.LittleEndian.AppendUint16(want, 4242) // ingredient without a template
	want = binary.LittleEndian.AppendUint16(want, 65535)
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = append(want, make([]byte, 8)...)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.AdenaID))
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryMoney))
	want = binary.LittleEndian.AppendUint32(want, 520)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = append(want, make([]byte, 8)...)
	if !bytes.Equal(got[:len(want)], want) {
		t.Fatalf("MultiSellList page 1 head =\n% x\nwant\n% x", got[:len(want)], want)
	}

	got = framePayload(t, FrameMultiSellList(list, 40))
	want = []byte{OpcodeMultiSellList}
	for _, v := range []int32{-7, 2, 1, 40, 1, 41, 0, 0} {
		want = binary.LittleEndian.AppendUint32(want, uint32(v))
	}
	want = append(want, 1) // an unknown product reads as stackable
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 4242)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 65535)
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = append(want, make([]byte, 8)...)
	want = binary.LittleEndian.AppendUint16(want, 1000)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = append(want, make([]byte, 8)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("MultiSellList page 2 =\n% x\nwant\n% x", got, want)
	}

	got = framePayload(t, FrameMultiSellList(&multisell.List{ID: 5}, 0))
	want = []byte{OpcodeMultiSellList}
	for _, v := range []int32{5, 1, 1, 40, 0} {
		want = binary.LittleEndian.AppendUint32(want, uint32(v))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("empty MultiSellList = % x, want % x", got, want)
	}
}
