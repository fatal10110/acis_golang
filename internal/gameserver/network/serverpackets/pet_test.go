package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func petPacketTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:        2375,
			Kind:      item.KindWeapon,
			Slot:      item.SlotWolf,
			Stackable: false,
			Weapon:    &item.WeaponDetail{Type: item.WeaponPet},
		},
		{
			ID:        57,
			Kind:      item.KindEtcItem,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
	})
}

func TestFramePetStatusShow(t *testing.T) {
	got := framePayload(t, FramePetStatusShow(2))
	want := []byte{OpcodePetStatusShow, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetStatusShow() = %x, want %x", got, want)
	}
}

func TestFramePetDelete(t *testing.T) {
	got := framePayload(t, FramePetDelete(2, 0x01020304))
	want := []byte{OpcodePetDelete, 0x02, 0x00, 0x00, 0x00, 0x04, 0x03, 0x02, 0x01}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetDelete() = %x, want %x", got, want)
	}
}

func TestFramePetInventoryUpdate(t *testing.T) {
	templates := petPacketTemplates()
	items := []*item.Instance{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, Location: item.LocationPet}}
	updates := []itemcontainer.Update{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, State: itemcontainer.UpdateModified}}

	frame, err := FramePetInventoryUpdate(updates, items, templates)
	if err != nil {
		t.Fatalf("FramePetInventoryUpdate: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{
		OpcodePetInventoryUpdate,
		0x01, 0x00,
		0x02, 0x00,
		0x04, 0x00,
		0x04, 0x03, 0x02, 0x01,
		0x39, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x04, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetInventoryUpdate() = %x, want %x", got, want)
	}
}
