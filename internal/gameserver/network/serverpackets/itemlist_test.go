package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// noManaLeft is the displayed-mana-left placeholder FrameItemList always
// writes (item.Instance carries no shadow-item duration state), kept as a
// variable so converting it to uint32 below is a runtime, not constant,
// conversion.
var noManaLeft int32 = -1

func TestFrameItemList(t *testing.T) {
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
	want = binary.LittleEndian.AppendUint16(want, 5) // enchant level
	want = binary.LittleEndian.AppendUint16(want, 0) // custom type 2
	want = binary.LittleEndian.AppendUint32(want, 0) // augmentation id
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

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
