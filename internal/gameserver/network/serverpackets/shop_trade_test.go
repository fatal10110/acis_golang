package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestFrameBuyList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
	})
	list := buylist.List{ID: 101, Products: []buylist.Product{
		{ItemID: 57, Price: 1, MaxCount: -1},
		{ItemID: 2368, Price: 625, MaxCount: 3},
	}}

	frame, err := FrameBuyList(list, 123456, 0.10, templates)
	if err != nil {
		t.Fatalf("FrameBuyList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeBuyList}
	want = binary.LittleEndian.AppendUint32(want, 123456)
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 57, 57, 0, item.SubCategoryMoney, item.SlotNone, 0, 0, 0, 1)
	want = appendShopTradeItem(want, item.CategoryWeaponOrJewelry, 2368, 2368, 3, item.SubCategoryWeapon, item.SlotLRHand, 0, 0, 0, 687)

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
	if _, err := FrameBuyList(buylist.List{ID: 1, Products: []buylist.Product{{ItemID: 9, MaxCount: -1}}}, 0, 0, item.NewTable(nil)); err == nil {
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
