package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestFrameGMViewItemList pins the full layout: the owner's name and limit,
// the window flag and count, then each item, a worn pet item flagged
// equipped and a carried one not.
func TestFrameGMViewItemList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: 60},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPetEquip, EnchantLevel: 3, CustomType1: 4, CustomType2: 6, ManaLeft: 125, Augmentation: &item.Augmentation{Attributes: 0x12345678}},
		{ObjectID: 101, TemplateID: item.AdenaID, Count: 500, Location: item.LocationPet},
	}

	frame, err := FrameGMViewItemList("Wolfie", 12, items, templates)
	if err != nil {
		t.Fatalf("FrameGMViewItemList: %v", err)
	}
	got := framePayload(t, frame)

	want := appendUTF16Z([]byte{OpcodeGMViewItemList}, "Wolfie")
	want = binary.LittleEndian.AppendUint32(want, 12) // limit
	want = binary.LittleEndian.AppendUint16(want, 1)  // show window
	want = binary.LittleEndian.AppendUint16(want, 2)  // item count

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryWeaponOrJewelry))
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 2368)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint16(want, 4) // custom type 1
	want = binary.LittleEndian.AppendUint16(want, 1) // equipped
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotLRHand))
	want = binary.LittleEndian.AppendUint16(want, 3)          // enchant level
	want = binary.LittleEndian.AppendUint16(want, 6)          // custom type 2
	want = binary.LittleEndian.AppendUint32(want, 0x12345678) // augmentation id
	want = binary.LittleEndian.AppendUint32(want, 2)          // displayed mana left

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryMoneyOrEtcItem))
	want = binary.LittleEndian.AppendUint32(want, 101)
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
		t.Errorf("FrameGMViewItemList mismatch:\n got  %x\n want %x", got, want)
	}
}

// TestFrameGMViewItemListErrorReturnsNoFrame pins a missing template as an
// error with no frame to release.
func TestFrameGMViewItemListErrorReturnsNoFrame(t *testing.T) {
	items := []*item.Instance{{ObjectID: 1, TemplateID: 999, Count: 1, Location: item.LocationPet}}
	frame, err := FrameGMViewItemList("", 12, items, item.NewTable(nil))
	if err == nil {
		t.Fatal("err = nil, want an error for a missing template")
	}
	frame.Release()
	if frame.Bytes() != nil {
		t.Errorf("frame.Bytes() = % X, want nil", frame.Bytes())
	}
}
