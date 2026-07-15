package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestUseEnchantScrollOpensSelection(t *testing.T) {
	templates := enchantTestTemplates()
	scroll := &item.Instance{ObjectID: 600, TemplateID: 955, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{scroll})
	gcl := &GameClientLink{}

	gcl.useItem(live, scroll.ObjectID)

	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != scroll.ObjectID {
		t.Fatalf("active enchant scroll = %d, want %d", got, scroll.ObjectID)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeChooseInventoryItem}) {
		t.Fatalf("opcodes = %x, want SystemMessage then ChooseInventoryItem", got)
	}
	assertStaticSystemMessageFrame(t, capture.frames[0], serverpackets.SystemMessageSelectItemToEnchant)
	assertChooseInventoryItemFrame(t, capture.frames[1], scroll.TemplateID)
}

func TestEnchantLiveItemSuccessConsumesScrollAndPersistsLevel(t *testing.T) {
	templates := enchantTestTemplates()
	weapon := &item.Instance{ObjectID: 500, TemplateID: 30, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	scroll := &item.Instance{ObjectID: 600, TemplateID: 955, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon, scroll})
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{items: store}
	gcl.enchantStateStore().Select(live.ObjectID(), scroll.ObjectID)

	gcl.enchantLiveItem(context.Background(), live, clientpackets.RequestEnchantItem{ObjectID: weapon.ObjectID})

	if weapon.EnchantLevel != 1 {
		t.Fatalf("weapon enchant = %d, want 1", weapon.EnchantLevel)
	}
	if live.Inventory().ItemByObjectID(scroll.ObjectID) != nil {
		t.Fatal("scroll still in inventory after successful enchant")
	}
	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active enchant scroll = %d, want cleared", got)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != weapon.ObjectID || store.updated[0].EnchantLevel != 1 {
		t.Fatalf("updated rows = %+v, want enchanted weapon", store.updated)
	}
	if len(store.deleted) != 1 || store.deleted[0] != scroll.ObjectID {
		t.Fatalf("deleted rows = %+v, want consumed scroll", store.deleted)
	}
	want := []byte{
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeInventoryUpdate,
		serverpackets.OpcodeEnchantResult,
		serverpackets.OpcodeUserInfo,
	}
	if got := frameOpcodes(capture.frames); string(got) != string(want) {
		t.Fatalf("opcodes = %x, want %x", got, want)
	}
	assertEnchantResultFrame(t, capture.frames[2], serverpackets.EnchantResultSuccess)
}

func TestEnchantLiveItemRejectsInvalidTargetWithoutConsumingScroll(t *testing.T) {
	templates := enchantTestTemplates()
	armor := &item.Instance{ObjectID: 501, TemplateID: 40, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	scroll := &item.Instance{ObjectID: 600, TemplateID: 955, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{armor, scroll})
	gcl := &GameClientLink{}
	gcl.enchantStateStore().Select(live.ObjectID(), scroll.ObjectID)

	gcl.enchantLiveItem(context.Background(), live, clientpackets.RequestEnchantItem{ObjectID: armor.ObjectID})

	if scroll.Count != 1 || live.Inventory().ItemByObjectID(scroll.ObjectID) == nil {
		t.Fatalf("scroll mutated on invalid enchant: %+v", scroll)
	}
	if armor.EnchantLevel != 0 {
		t.Fatalf("armor enchant = %d, want unchanged", armor.EnchantLevel)
	}
	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active enchant scroll = %d, want cleared", got)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeEnchantResult}) {
		t.Fatalf("opcodes = %x, want SystemMessage then EnchantResult", got)
	}
	assertStaticSystemMessageFrame(t, capture.frames[0], serverpackets.SystemMessageInappropriateEnchantCondition)
	assertEnchantResultFrame(t, capture.frames[1], serverpackets.EnchantResultCancelled)
}

func TestEnchantLiveItemFailureBreaksItemIntoCrystals(t *testing.T) {
	templates := enchantTestTemplates()
	weapon := &item.Instance{ObjectID: 500, TemplateID: 30, OwnerID: 1, Count: 1, EnchantLevel: 3, Location: item.LocationInventory}
	scroll := &item.Instance{ObjectID: 600, TemplateID: 955, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon, scroll})
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{items: store, ids: &sequentialIDs{next: 700}, enchantRoll: func() float64 { return 0.99 }}
	gcl.enchantStateStore().Select(live.ObjectID(), scroll.ObjectID)

	gcl.enchantLiveItem(context.Background(), live, clientpackets.RequestEnchantItem{ObjectID: weapon.ObjectID})

	if live.Inventory().ItemByObjectID(weapon.ObjectID) != nil {
		t.Fatal("failed normal enchant left source weapon in inventory")
	}
	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active enchant scroll = %d, want cleared", got)
	}
	crystals := live.Inventory().ItemByTemplateID(item.CrystalD.ItemID())
	if crystals == nil || crystals.Count != 275 {
		t.Fatalf("crystals = %+v, want 275 D crystals", crystals)
	}
	if len(store.deleted) != 2 || store.deleted[0] != scroll.ObjectID || store.deleted[1] != weapon.ObjectID {
		t.Fatalf("deleted rows = %+v, want scroll then weapon", store.deleted)
	}
	if len(store.saved) != 1 || store.saved[0].TemplateID != item.CrystalD.ItemID() || store.saved[0].Count != 275 {
		t.Fatalf("saved rows = %+v, want crystal reward", store.saved)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeInventoryUpdate,
		serverpackets.OpcodeEnchantResult,
		serverpackets.OpcodeUserInfo,
	}) {
		t.Fatalf("opcodes = %x, want crystal/system messages, inventory, result, UserInfo", got)
	}
	assertEnchantResultFrame(t, capture.frames[3], serverpackets.EnchantResultBrokenWithCrystals)
}

func TestEnchantLiveItemBlessedFailureResetsEnchantLevel(t *testing.T) {
	templates := enchantTestTemplates()
	weapon := &item.Instance{ObjectID: 500, TemplateID: 30, OwnerID: 1, Count: 1, EnchantLevel: 3, Location: item.LocationInventory}
	scroll := &item.Instance{ObjectID: 601, TemplateID: 6575, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon, scroll})
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{items: store, enchantRoll: func() float64 { return 0.99 }}
	gcl.enchantStateStore().Select(live.ObjectID(), scroll.ObjectID)

	gcl.enchantLiveItem(context.Background(), live, clientpackets.RequestEnchantItem{ObjectID: weapon.ObjectID})

	if weapon.EnchantLevel != 0 {
		t.Fatalf("weapon enchant = %d, want reset to 0", weapon.EnchantLevel)
	}
	if live.Inventory().ItemByObjectID(weapon.ObjectID) == nil {
		t.Fatal("blessed failure destroyed source weapon")
	}
	if live.Inventory().ItemByObjectID(scroll.ObjectID) != nil {
		t.Fatal("blessed failure left scroll in inventory")
	}
	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active enchant scroll = %d, want cleared", got)
	}
	if len(store.deleted) != 1 || store.deleted[0] != scroll.ObjectID {
		t.Fatalf("deleted rows = %+v, want consumed blessed scroll", store.deleted)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != weapon.ObjectID || store.updated[0].EnchantLevel != 0 {
		t.Fatalf("updated rows = %+v, want reset weapon", store.updated)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeInventoryUpdate,
		serverpackets.OpcodeEnchantResult,
		serverpackets.OpcodeUserInfo,
	}) {
		t.Fatalf("opcodes = %x, want blessed message, inventory, result, UserInfo", got)
	}
	assertStaticSystemMessageFrame(t, capture.frames[0], serverpackets.SystemMessageBlessedEnchantFailed)
	assertEnchantResultFrame(t, capture.frames[2], serverpackets.EnchantResultUnsuccess)
}

func enchantTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:           30,
			Name:         "Sword",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalD,
			CrystalCount: 10,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword},
		},
		{
			ID:       40,
			Name:     "Tunic",
			Kind:     item.KindArmor,
			Slot:     item.SlotChest,
			Duration: -1,
			Crystal:  item.CrystalD,
			Armor:    &item.ArmorDetail{Type: item.ArmorMagic},
		},
		{
			ID:        955,
			Name:      "Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:        6575,
			Name:      "Blessed Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemBlessedScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:        item.CrystalD.ItemID(),
			Name:      "D-grade Crystal",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
	})
}

func assertStaticSystemMessageFrame(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	if len(frame) != 9 || frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("SystemMessage frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("system message params = %d, want 0", got)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

func assertChooseInventoryItemFrame(t *testing.T, frame []byte, itemID int32) {
	t.Helper()
	if len(frame) != 5 || frame[0] != serverpackets.OpcodeChooseInventoryItem {
		t.Fatalf("ChooseInventoryItem frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("ChooseInventoryItem item id = %d, want %d", got, itemID)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ChooseInventoryItem: %v", err)
	}
}

func assertEnchantResultFrame(t *testing.T, frame []byte, result serverpackets.EnchantResult) {
	t.Helper()
	if len(frame) != 5 || frame[0] != serverpackets.OpcodeEnchantResult {
		t.Fatalf("EnchantResult frame = %x", frame)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(result) {
		t.Fatalf("EnchantResult result = %d, want %d", got, result)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read EnchantResult: %v", err)
	}
}

type recordingEnchantItemStore struct {
	updated []item.Instance
	saved   []item.Instance
	deleted []int32
}

func (s *recordingEnchantItemStore) ListByOwner(context.Context, int32) ([]*item.Instance, error) {
	return nil, nil
}

func (s *recordingEnchantItemStore) Update(_ context.Context, inst *item.Instance) error {
	s.updated = append(s.updated, *inst)
	return nil
}

func (s *recordingEnchantItemStore) Save(_ context.Context, inst *item.Instance) error {
	s.saved = append(s.saved, *inst)
	return nil
}

func (s *recordingEnchantItemStore) Delete(_ context.Context, objectID int32) error {
	s.deleted = append(s.deleted, objectID)
	return nil
}
