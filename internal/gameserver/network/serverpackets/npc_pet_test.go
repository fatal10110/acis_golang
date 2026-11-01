package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

// ---- from npcsay_test.go ----
func TestFrameNpcSay(t *testing.T) {
	got := framePayload(t, FrameNpcSay(500, 12564, SayTypeAll, "Hello"))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 500)
	want = appendD(want, 0)
	want = appendD(want, 1012564)
	want = append(want, encodeUTF16Z("Hello")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}

// TestFrameNpcSayResolvesNpcStringId pins issue #2028: a walkerRoutes.xml
// node's fstring resolves through the ported npcstring table (aCis
// NpcStringId.getMessage()) before broadcasting via NpcSay.
func TestFrameNpcSayResolvesNpcStringId(t *testing.T) {
	text, ok := npcstring.Text(3)
	if !ok || text != "Opening" {
		t.Fatalf("npcstring.Text(3) = (%q, %v), want (\"Opening\", true)", text, ok)
	}

	got := framePayload(t, FrameNpcSay(12345, 31357, SayTypeAll, text))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 12345)
	want = appendD(want, 0)
	want = appendD(want, 1_000_000+31357)
	want = append(want, encodeUTF16Z(text)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}

// ---- from pet_test.go ----
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

func TestFramePetItemList(t *testing.T) {
	items := []*item.Instance{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, Location: item.LocationPet}}
	frame, err := FramePetItemList(items, petPacketTemplates())
	if err != nil {
		t.Fatalf("FramePetItemList: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{
		OpcodePetItemList,
		0x01, 0x00,
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
		t.Fatalf("FramePetItemList() = %x, want %x", got, want)
	}
}

func TestFramePetItemListRejectsOversizedCount(t *testing.T) {
	items := make([]*item.Instance, 1<<16)
	if _, err := FramePetItemList(items, item.NewTable(nil)); err == nil {
		t.Fatal("FramePetItemList oversized count error = nil, want error")
	}
}

func TestFramePetInventoryUpdate(t *testing.T) {
	templates := petPacketTemplates()
	updates := []itemcontainer.Update{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, State: itemcontainer.UpdateModified}}

	frame, err := FramePetInventoryUpdate(updates, templates)
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

// ---- from ride_test.go ----
