package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// ---- from enchant_test.go ----
func TestFrameEnchantResult(t *testing.T) {
	got := framePayload(t, FrameEnchantResult(EnchantResultCancelled))
	want := []byte{OpcodeEnchantResult, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameEnchantResult() = %x, want %x", got, want)
	}
}

func TestFrameChooseInventoryItem(t *testing.T) {
	got := framePayload(t, FrameChooseInventoryItem(955))
	want := []byte{OpcodeChooseInventoryItem, 0xbb, 0x03, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChooseInventoryItem() = %x, want %x", got, want)
	}
}

// ---- from exautosoulshot_test.go ----
func TestFrameExAutoSoulShot(t *testing.T) {
	got := framePayload(t, FrameExAutoSoulShot(1463, true))
	want := []byte{
		OpcodeExtended,
		0x12, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExAutoSoulShot() = %x, want %x", got, want)
	}
}

func TestFrameExUseSharedGroupItem(t *testing.T) {
	got := framePayload(t, FrameExUseSharedGroupItem(1463, 4, 12_000, 60_000))
	want := []byte{
		OpcodeExtended,
		0x49, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x04, 0x00, 0x00, 0x00,
		0x0c, 0x00, 0x00, 0x00,
		0x3c, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExUseSharedGroupItem() = %x, want %x", got, want)
	}
}

func TestFrameItemListErrorReturnsNoFrame(t *testing.T) {
	items := []*item.Instance{{ObjectID: 1, TemplateID: 999, Count: 1, Location: item.LocationInventory}}

	frame, err := FrameItemList(items, item.NewTable(nil), true)
	if err == nil {
		t.Fatal("FrameItemList err = nil, want an error for a missing template")
	}
	frame.Release() // must be a no-op on the zero frame
	if frame.Bytes() != nil {
		t.Errorf("frame.Bytes() = % X, want nil", frame.Bytes())
	}
}

// ---- from grounditems_test.go ----
func TestFrameSpawnItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 57, count: 500, stackable: true, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameSpawnItem(ground))

	want := []byte{OpcodeSpawnItem}
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 57)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 500)
	want = binary.LittleEndian.AppendUint32(want, 0)
	if string(got) != string(want) {
		t.Fatalf("FrameSpawnItem() = % x, want % x", got, want)
	}
}

func TestFrameDropItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 10, count: 1, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameDropItem(ground, 200))

	want := []byte{OpcodeDropItem}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	if string(got) != string(want) {
		t.Fatalf("FrameDropItem() = % x, want % x", got, want)
	}
}

func TestFrameGetItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 57, count: 500, stackable: true, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameGetItem(ground, 200))

	want := []byte{OpcodeGetItem}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	if string(got) != string(want) {
		t.Fatalf("FrameGetItem() = % x, want % x", got, want)
	}
}

func appendInt32(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

type packetGroundItem struct {
	id, itemID int32
	count      int
	stackable  bool
	x, y, z    int
}

func (p packetGroundItem) ObjectID() int32 { return p.id }

func (p packetGroundItem) ItemID() int32 { return p.itemID }

func (p packetGroundItem) Count() int { return p.count }

func (p packetGroundItem) Stackable() bool { return p.stackable }

func (p packetGroundItem) Position() (int, int, int) {
	return p.x, p.y, p.z
}

// ---- from inventoryupdate_test.go ----
func TestFrameInventoryUpdate(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5, CustomType1: 3, CustomType2: 4, ManaLeft: -1, Augmentation: &item.Augmentation{Attributes: 777}},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationInventory, ManaLeft: -1},
	}
	updates := []itemcontainer.Update{
		{ObjectID: 100, TemplateID: 2368, Count: 1, State: itemcontainer.UpdateModified},
		{ObjectID: 101, TemplateID: 1146, Count: 1, State: itemcontainer.UpdateRemoved},
	}

	frame, err := FrameInventoryUpdate(updates, items, templates)
	if err != nil {
		t.Fatalf("FrameInventoryUpdate: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeInventoryUpdate}
	want = binary.LittleEndian.AppendUint16(want, 2)

	want = binary.LittleEndian.AppendUint16(want, uint16(itemcontainer.UpdateModified))
	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryWeaponOrJewelry))
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 2368)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotLRHand))
	want = binary.LittleEndian.AppendUint16(want, 5)
	want = binary.LittleEndian.AppendUint16(want, 4)
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	want = binary.LittleEndian.AppendUint16(want, uint16(itemcontainer.UpdateRemoved))
	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryArmor))
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 1146)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryArmor))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotChest))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	if !bytes.Equal(got, want) {
		t.Errorf("FrameInventoryUpdate mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameInventoryUpdateMissingTemplate(t *testing.T) {
	_, err := FrameInventoryUpdate([]itemcontainer.Update{{ObjectID: 1, TemplateID: 999, Count: 1}}, nil, item.NewTable(nil))
	if err == nil {
		t.Fatal("FrameInventoryUpdate: want error for missing template")
	}
}

// ---- from itemlist_test.go ----
var noManaLeft int32 = -1

func TestFrameItemList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: 60},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5, ManaLeft: 125, Augmentation: &item.Augmentation{Attributes: 0x12345678}},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll, LocationData: 10},
		{ObjectID: 102, TemplateID: item.AdenaID, Count: 500, Location: item.LocationInventory},
		{ObjectID: 103, TemplateID: 1146, Count: 1, Location: item.LocationWarehouse}, // excluded: not carried
	}

	frame, err := FrameItemList(items, templates, true)
	if err != nil {
		t.Fatalf("FrameItemList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeItemList}
	want = binary.LittleEndian.AppendUint16(want, 1) // show window
	want = binary.LittleEndian.AppendUint16(want, 3) // carried item count

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryWeaponOrJewelry))
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 2368)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint16(want, 0) // custom type 1
	want = binary.LittleEndian.AppendUint16(want, 1) // equipped
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotLRHand))
	want = binary.LittleEndian.AppendUint16(want, 5)          // enchant level
	want = binary.LittleEndian.AppendUint16(want, 0)          // custom type 2
	want = binary.LittleEndian.AppendUint32(want, 0x12345678) // augmentation id
	want = binary.LittleEndian.AppendUint32(want, 2)          // displayed mana left

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryArmor))
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 1146)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryArmor))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotChest))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryMoneyOrEtcItem))
	want = binary.LittleEndian.AppendUint32(want, 102)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.AdenaID))
	want = binary.LittleEndian.AppendUint32(want, 500)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryMoney))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0) // not equipped
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotNone))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	if !bytes.Equal(got, want) {
		t.Errorf("FrameItemList mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameItemList_HideWindow(t *testing.T) {
	frame, err := FrameItemList(nil, item.NewTable(nil), false)
	if err != nil {
		t.Fatalf("FrameItemList: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{OpcodeItemList, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("FrameItemList (empty, hidden) = %x, want %x", got, want)
	}
}

// TestWriteItemListNoAuxiliaryAllocation guards against writeItemList
// reintroducing a filtered snapshot slice: with a pre-sized Writer and
// already-locked item.Instances, encoding must not allocate.
func TestWriteItemListNoAuxiliaryAllocation(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll, LocationData: 10},
		{ObjectID: 102, TemplateID: item.AdenaID, Count: 500, Location: item.LocationInventory},
		{ObjectID: 103, TemplateID: 1146, Count: 1, Location: item.LocationWarehouse}, // excluded: not carried
	}

	var w wire.Writer
	w.ResetFrame(256) // pre-size so append growth doesn't allocate during the measured run
	if err := writeItemList(&w, items, templates, true); err != nil {
		t.Fatalf("writeItemList: %v", err) // warm up per-item mutex lazy-init before measuring
	}
	warm := append([]byte(nil), w.Bytes()...)

	allocs := testing.AllocsPerRun(100, func() {
		w.ResetFrame(256)
		if err := writeItemList(&w, items, templates, true); err != nil {
			t.Fatalf("writeItemList: %v", err)
		}
	})
	if allocs != 0 {
		t.Errorf("writeItemList allocs/run = %v, want 0 (no auxiliary filter/snapshot slice)", allocs)
	}

	w.ResetFrame(256)
	if err := writeItemList(&w, items, templates, true); err != nil {
		t.Fatalf("writeItemList: %v", err)
	}
	if !bytes.Equal(w.Bytes(), warm) {
		t.Errorf("writeItemList output changed across repeated runs:\n got  %x\n want %x", w.Bytes(), warm)
	}
}

func TestFrameExShowVariationWindows(t *testing.T) {
	got := framePayload(t, FrameExShowVariationMakeWindow())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationMakeWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationMakeWindow() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameExShowVariationCancelWindow())
	want = []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationCancelWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationCancelWindow() = %x, want %x", got, want)
	}
}

// TestFrameExShowQuestInfo pins the quest information window: writeC(0xfe)
// then writeH(0x19), nothing more (ExShowQuestInfo.java).
func TestFrameExShowQuestInfo(t *testing.T) {
	if got, want := framePayload(t, FrameExShowQuestInfo()), []byte{0xfe, 0x19, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowQuestInfo() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationItem(1000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationItem)
	want = appendD(want, 1000)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationItem() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationRefiner(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationRefiner(2000, 8723, 2130, 20))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationRefiner)
	want = appendD(want, 2000)
	want = appendD(want, 8723)
	want = appendD(want, 2130)
	want = appendD(want, 20)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationRefiner() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationGemstone(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationGemstone(3000, 36))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationGemstone)
	want = appendD(want, 3000)
	want = appendD(want, 1)
	want = appendD(want, 36)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationGemstone() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmCancelItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmCancelItem(1000, 7575, 0x12345678, 390000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmCancelItem)
	want = appendD(want, 1000)
	want = appendD(want, 7575)
	want = appendD(want, 0x5678)
	want = appendD(want, 0x1234)
	want = appendQ(want, 390000)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmCancelItem() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResult(t *testing.T) {
	got := framePayload(t, FrameExVariationResult(0x1111, 0x2222, 1))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0x1111)
	want = appendD(want, 0x2222)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResult() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResultFailed(t *testing.T) {
	got := framePayload(t, FrameExVariationResultFailed())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResultFailed() = %x, want %x", got, want)
	}
}

func TestFrameExVariationCancelResult(t *testing.T) {
	got := framePayload(t, FrameExVariationCancelResult(1))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationCancelResult)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationCancelResult() = %x, want %x", got, want)
	}
}

// ---- from warehouse_test.go ----
func TestFramePackageToList(t *testing.T) {
	got := framePayload(t, FramePackageToList([]PackageRecipient{
		{ObjectID: 100, Name: "Alpha"},
		{ObjectID: 200, Name: "Beta"},
	}))

	want := []byte{OpcodePackageToList}
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = appendUTF16Z(want, "Alpha")
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = appendUTF16Z(want, "Beta")

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePackageToList = %x, want %x", got, want)
	}
}

func TestFramePackageSendableList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 30, Count: 1, Location: item.LocationInventory, EnchantLevel: 2, CustomType1: 7, CustomType2: 8},
		{ObjectID: 501, TemplateID: item.AdenaID, Count: 100, Location: item.LocationInventory},
	}

	frame, err := FramePackageSendableList(200, 777, items, templates)
	if err != nil {
		t.Fatalf("FramePackageSendableList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodePackageSendableList}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = appendWarehouseVisibleItem(want, items[0], templates, false)
	want = appendWarehouseVisibleItem(want, items[1], templates, false)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePackageSendableList = %x, want %x", got, want)
	}
}

func TestFrameWarehouseDepositList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{
		{
			ObjectID: 500, TemplateID: 30, Count: 1, Location: item.LocationInventory,
			EnchantLevel: 2, CustomType1: 7, CustomType2: 8,
			Augmentation: &item.Augmentation{Attributes: 0x12345678},
		},
	}

	frame, err := FrameWarehouseDepositList(WarehousePrivate, 777, items, templates)
	if err != nil {
		t.Fatalf("FrameWarehouseDepositList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeWarehouseDepositList}
	want = binary.LittleEndian.AppendUint16(want, uint16(WarehousePrivate))
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = appendWarehouseVisibleItem(want, items[0], templates, true)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameWarehouseDepositList = %x, want %x", got, want)
	}
}

func TestFrameWarehouseWithdrawList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{{ObjectID: 501, TemplateID: item.AdenaID, Count: 100, Location: item.LocationWarehouse}}

	frame, err := FrameWarehouseWithdrawList(WarehouseFreight, 777, items, templates)
	if err != nil {
		t.Fatalf("FrameWarehouseWithdrawList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeWarehouseWithdrawList}
	want = binary.LittleEndian.AppendUint16(want, uint16(WarehouseFreight))
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = appendWarehouseVisibleItem(want, items[0], templates, true)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameWarehouseWithdrawList = %x, want %x", got, want)
	}
}

func TestFramePackageSendableListMissingTemplate(t *testing.T) {
	_, err := FramePackageSendableList(200, 777, []*item.Instance{{ObjectID: 500, TemplateID: 999, Count: 1}}, item.NewTable(nil))
	if err == nil {
		t.Fatal("FramePackageSendableList: want error for missing template")
	}
}

func warehousePacketTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1, Weapon: &item.WeaponDetail{}},
	})
}

func appendWarehouseVisibleItem(out []byte, inst *item.Instance, templates *item.Table, includeAugmentation bool) []byte {
	tmpl, _ := templates.Get(inst.TemplateID)
	category, subCategory := tmpl.Category()

	out = binary.LittleEndian.AppendUint16(out, uint16(category))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.ObjectID))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.TemplateID))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.Count))
	out = binary.LittleEndian.AppendUint16(out, uint16(subCategory))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.CustomType1))
	out = binary.LittleEndian.AppendUint32(out, uint32(tmpl.Slot))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.EnchantLevel))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.CustomType2))
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.ObjectID))
	if includeAugmentation {
		if inst.Augmentation != nil {
			out = binary.LittleEndian.AppendUint32(out, uint32(inst.Augmentation.Attributes&0x0000ffff))
			return binary.LittleEndian.AppendUint32(out, uint32(inst.Augmentation.Attributes>>16))
		}
		return binary.LittleEndian.AppendUint64(out, 0)
	}
	return out
}

func appendUTF16Z(out []byte, s string) []byte {
	for _, unit := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}
