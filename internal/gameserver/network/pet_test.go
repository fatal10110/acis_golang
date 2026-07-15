package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func petTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:          item.AdenaID,
			Name:        "Adena",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
		{
			ID:          2375,
			Name:        "Wolf Tooth",
			Kind:        item.KindWeapon,
			Slot:        item.SlotWolf,
			Duration:    -1,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Weapon:      &item.WeaponDetail{Type: item.WeaponPet},
		},
		{
			ID:          9000,
			Name:        "Forbidden",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    false,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
	})
}

func attachTestPet(t *testing.T, state *world.State, live *livePlayer, templates *item.Table, npcID int, items []*item.Instance) (*summon.Actor, *itemcontainer.Inventory) {
	t.Helper()
	petInv := itemcontainer.NewPetInventory(0x20000001, templates)
	petInv.Restore(items)
	pet := summon.NewPet(summon.PetConfig{
		ObjectID:  0x20000001,
		Owner:     live,
		NPCID:     npcID,
		Level:     1,
		Inventory: petInv,
		Fed:       100,
		MaxMeal:   100,
	})
	summon.SpawnBesideOwner(state, pet, live, location.Location{X: 10})
	return pet, petInv
}

func TestGiveItemToPetTransfersAndPersists(t *testing.T) {
	templates := petTestTemplates()
	source := &item.Instance{ObjectID: 500, TemplateID: item.AdenaID, OwnerID: 1, Count: 100, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{source})
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	_, petInv := attachTestPet(t, state, live, templates, 12077, nil)
	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, ids: &sequentialIDs{next: 900}, items: store}

	gcl.giveItemToPet(context.Background(), live, clientpackets.RequestGiveItemToPet{ObjectID: source.ObjectID, Count: 30})

	if source.Count != 70 {
		t.Fatalf("source Count = %d, want 70", source.Count)
	}
	petStack := petInv.ItemByTemplateID(item.AdenaID)
	if petStack == nil || petStack.Count != 30 || petStack.OwnerID != 0x20000001 || petStack.Location != item.LocationPet {
		t.Fatalf("pet stack = %+v, want 30 adena in pet inventory", petStack)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeInventoryUpdate, serverpackets.OpcodePetInventoryUpdate}) {
		t.Fatalf("opcodes = %x, want InventoryUpdate then PetInventoryUpdate", got)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != source.ObjectID || store.updated[0].Count != 70 {
		t.Fatalf("updated rows = %+v, want reduced source stack", store.updated)
	}
	if len(store.saved) != 1 || store.saved[0].ObjectID != petStack.ObjectID || store.saved[0].Count != 30 || store.saved[0].OwnerID != 0x20000001 || store.saved[0].Location != item.LocationPet {
		t.Fatalf("saved rows = %+v, want new pet stack", store.saved)
	}
}

func TestGetItemFromPetTransfersBackToOwner(t *testing.T) {
	templates := petTestTemplates()
	petItem := &item.Instance{ObjectID: 600, TemplateID: item.AdenaID, OwnerID: 0x20000001, Count: 40, Location: item.LocationPet}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	_, petInv := attachTestPet(t, state, live, templates, 12077, []*item.Instance{petItem})
	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, ids: &sequentialIDs{next: 910}, items: store}

	gcl.getItemFromPet(context.Background(), live, clientpackets.RequestGetItemFromPet{ObjectID: petItem.ObjectID, Count: 15})

	if petItem.Count != 25 {
		t.Fatalf("pet item Count = %d, want 25", petItem.Count)
	}
	playerStack := live.Inventory().ItemByTemplateID(item.AdenaID)
	if playerStack == nil || playerStack.Count != 15 || playerStack.OwnerID != live.ObjectID() || playerStack.Location != item.LocationInventory {
		t.Fatalf("player stack = %+v, want 15 adena in player inventory", playerStack)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodeInventoryUpdate}) {
		t.Fatalf("opcodes = %x, want PetInventoryUpdate then InventoryUpdate", got)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != petItem.ObjectID || store.updated[0].Count != 25 {
		t.Fatalf("updated rows = %+v, want reduced pet stack", store.updated)
	}
	if len(store.saved) != 1 || store.saved[0].ObjectID != playerStack.ObjectID || store.saved[0].Count != 15 || store.saved[0].OwnerID != live.ObjectID() || store.saved[0].Location != item.LocationInventory {
		t.Fatalf("saved rows = %+v, want new player stack", store.saved)
	}
	_ = petInv
}

func TestGiveItemToPetCancelsActiveEnchantBeforeTransfer(t *testing.T) {
	templates := petTestTemplates()
	source := &item.Instance{ObjectID: 500, TemplateID: item.AdenaID, OwnerID: 1, Count: 100, Location: item.LocationInventory}
	scroll := &item.Instance{ObjectID: 501, TemplateID: 955, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{source, scroll})
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	attachTestPet(t, state, live, templates, 12077, nil)
	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, ids: &sequentialIDs{next: 900}, items: store}
	gcl.enchantStateStore().Select(live.ObjectID(), scroll.ObjectID)

	gcl.giveItemToPet(context.Background(), live, clientpackets.RequestGiveItemToPet{ObjectID: source.ObjectID, Count: 30})

	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active enchant scroll = %d, want cleared", got)
	}
	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeEnchantResult,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeInventoryUpdate,
		serverpackets.OpcodePetInventoryUpdate,
	)
	assertEnchantResultFrame(t, capture.frames[0], serverpackets.EnchantResultCancelled)
	assertStaticSystemMessageFrame(t, capture.frames[1], serverpackets.SystemMessageEnchantScrollCancelled)
}

func TestPetUseItemEquipsWolfWeapon(t *testing.T) {
	templates := petTestTemplates()
	weapon := &item.Instance{ObjectID: 700, TemplateID: 2375, OwnerID: 0x20000001, Count: 1, Location: item.LocationPet}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	_, petInv := attachTestPet(t, state, live, templates, 12077, []*item.Instance{weapon})
	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, items: store}

	gcl.petUseItem(context.Background(), live, clientpackets.RequestPetUseItem{ObjectID: weapon.ObjectID})

	if weapon.Location != item.LocationPetEquip || weapon.LocationData != itemcontainer.RHand || petInv.ItemAt(itemcontainer.RHand) != weapon {
		t.Fatalf("weapon equip state = %+v, want pet RHand equipped", weapon)
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate}) {
		t.Fatalf("opcodes = %x, want SystemMessage then PetInventoryUpdate", got)
	}
	assertSystemMessageItemFrame(t, capture.frames[0], serverpackets.SystemMessagePetPutOnS1, weapon.TemplateID)
	if len(store.updated) != 1 || store.updated[0].ObjectID != weapon.ObjectID || store.updated[0].Location != item.LocationPetEquip {
		t.Fatalf("updated rows = %+v, want equipped pet weapon", store.updated)
	}
}

func TestGiveItemToPetRejectsForbiddenItem(t *testing.T) {
	templates := petTestTemplates()
	source := &item.Instance{ObjectID: 800, TemplateID: 9000, OwnerID: 1, Count: 1, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{source})
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	attachTestPet(t, state, live, templates, 12077, nil)
	capture.frames = nil
	gcl := &GameClientLink{world: state, ids: &sequentialIDs{next: 900}}

	gcl.giveItemToPet(context.Background(), live, clientpackets.RequestGiveItemToPet{ObjectID: source.ObjectID, Count: 1})

	if live.Inventory().ItemByObjectID(source.ObjectID) == nil {
		t.Fatal("forbidden item moved out of player inventory")
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("opcodes = %x, want SystemMessage only", got)
	}
	assertStaticSystemMessageFrame(t, capture.frames[0], serverpackets.SystemMessageItemNotForPets)
}

func TestGameClientLinkRequestGiveItemToPetDispatch(t *testing.T) {
	c, chars, items, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   500,
		TemplateID: item.AdenaID,
		OwnerID:    objID,
		Count:      100,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerObj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	live := playerObj.(*livePlayer)
	_, petInv := attachTestPet(t, state, live, testItemTemplates(), 12077, nil)

	c.send(encodeRequestGiveItemToPet(500, 25))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("first reply opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodePetInventoryUpdate {
		t.Fatalf("second reply opcode = %#x, want PetInventoryUpdate (%#x)", reply[0], serverpackets.OpcodePetInventoryUpdate)
	}
	if stack := petInv.ItemByTemplateID(item.AdenaID); stack == nil || stack.Count != 25 {
		t.Fatalf("pet stack = %+v, want 25 adena", stack)
	}
}

func TestHandleTargetActionShowsPetStatusForOwnerPet(t *testing.T) {
	templates := petTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, _ := attachTestPet(t, state, live, templates, 12077, nil)
	capture.frames = nil
	gcl := &GameClientLink{world: state}

	gcl.handleTargetAction(live, pet.ObjectID(), false)
	capture.frames = nil
	gcl.handleTargetAction(live, pet.ObjectID(), true)

	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodePetStatusShow}) {
		t.Fatalf("opcodes = %x, want PetStatusShow", got)
	}
	r := wire.NewReader(capture.frames[0][1:])
	if got := r.ReadInt32(); got != int32(pet.SummonType()) {
		t.Fatalf("PetStatusShow summon type = %d, want %d", got, pet.SummonType())
	}
}

func encodeRequestGiveItemToPet(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGiveItemToPet)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}
