package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func newEquipTestLivePlayer(t *testing.T, id int32, capture *frameCapture, templates *item.Table, items []*item.Instance) *livePlayer {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch := &player.Character{
		ID: id, Name: "Player", ClassID: 0, BaseClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		Level: 1, MaxHP: 80, CurHP: 80, MaxMP: 30, CurMP: 30,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, templates, items))
	ch.SetFrameSender(capture.send)
	return &livePlayer{Character: ch, template: tmpl, items: items}
}

func TestUseItemTogglesEquipState(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}}})
	weapon := &item.Instance{ObjectID: 500, TemplateID: 10, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
	gcl := &GameClientLink{}

	gcl.useItem(live, weapon.ObjectID)

	if !weapon.Equipped() {
		t.Fatal("weapon not equipped after first UseItem")
	}
	if weapon.Location != item.LocationPaperdoll || weapon.LocationData != itemcontainer.RHand {
		t.Fatalf("weapon location = %v/%d, want paperdoll/RHand", weapon.Location, weapon.LocationData)
	}
	if len(capture.frames) != 2 || capture.frames[0][0] != serverpackets.OpcodeInventoryUpdate || capture.frames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("frames after equip = %x, want InventoryUpdate then UserInfo", capture.frames)
	}
	capture.frames = nil

	gcl.useItem(live, weapon.ObjectID)

	if weapon.Equipped() {
		t.Fatal("weapon still equipped after second UseItem")
	}
	if weapon.Location != item.LocationInventory {
		t.Fatalf("weapon location = %v, want inventory", weapon.Location)
	}
	if len(capture.frames) != 2 || capture.frames[0][0] != serverpackets.OpcodeInventoryUpdate || capture.frames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("frames after unequip = %x, want InventoryUpdate then UserInfo", capture.frames)
	}
}

func TestUseItemUnknownObjectIDIsNoop(t *testing.T) {
	templates := item.NewTable(nil)
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	gcl := &GameClientLink{}

	gcl.useItem(live, 999)

	if len(capture.frames) != 0 {
		t.Fatalf("frames for unknown object id = %x, want none", capture.frames)
	}
}

func TestUnequipItemBySlot(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}}})
	chest := &item.Instance{ObjectID: 501, TemplateID: 20, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{chest})
	gcl := &GameClientLink{}

	gcl.unequipItem(live, int32(item.SlotChest))

	if chest.Equipped() {
		t.Fatal("chest piece still equipped after RequestUnEquipItem")
	}
	if len(capture.frames) != 2 || capture.frames[0][0] != serverpackets.OpcodeInventoryUpdate || capture.frames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("frames after unequip = %x, want InventoryUpdate then UserInfo", capture.frames)
	}
}

func TestUnequipItemEmptySlotIsNoop(t *testing.T) {
	templates := item.NewTable(nil)
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	gcl := &GameClientLink{}

	gcl.unequipItem(live, int32(item.SlotChest))

	if len(capture.frames) != 0 {
		t.Fatalf("frames for empty slot = %x, want none", capture.frames)
	}
}

func TestUseItemBroadcastsCharInfoToObservers(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}}})
	weapon := &item.Instance{ObjectID: 500, TemplateID: 10, Location: item.LocationInventory}
	state := world.New()
	wearerFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	wearer := newEquipTestLivePlayer(t, 1, wearerFrames, templates, []*item.Instance{weapon})
	observer := newEquipTestLivePlayer(t, 2, observerFrames, item.NewTable(nil), nil)

	state.Spawn(wearer, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	wearerFrames.frames = nil
	observerFrames.frames = nil

	gcl := &GameClientLink{world: state}
	gcl.useItem(wearer, weapon.ObjectID)

	if len(wearerFrames.frames) != 2 || wearerFrames.frames[0][0] != serverpackets.OpcodeInventoryUpdate || wearerFrames.frames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("wearer frames = %x, want InventoryUpdate then UserInfo", wearerFrames.frames)
	}
	if len(observerFrames.frames) != 1 || observerFrames.frames[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("observer frames = %x, want one CharInfo", observerFrames.frames)
	}
}
