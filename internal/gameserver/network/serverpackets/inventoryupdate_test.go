package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

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
