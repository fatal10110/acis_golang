package network

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/petitem"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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
	// The dial-based tests already have a batching task registered for
	// state (from the login flow) by the time they attach a pet; wire the
	// pet's inventory into it here, the same attach-point wiring newPet
	// does in production. Tests that build *GameClientLink directly instead
	// run this before the task exists — lookup finds nothing yet, and
	// wireInventoryUpdates picks the pet up once it does.
	if updates, ok := lookupTestInventoryUpdates(state); ok {
		wirePetInventoryUpdates(updates, pet, live, zerolog.Nop())
	}
	return pet, petInv
}

func TestGameClientLinkNewPetAppliesConfiguredPetLimits(t *testing.T) {
	cfg := petmodel.DefaultConfig()
	cfg.ExpRate = 1.75
	cfg.InventorySlots = 9
	cfg.WeightLimitMultiplier = 2.0

	inventory := itemcontainer.NewPetInventory(0x20000001, petTestTemplates())
	gcl := &GameClientLink{petConfig: cfg}

	pet := gcl.newPet(summon.PetConfig{
		ObjectID:  0x20000001,
		NPCID:     12077,
		CON:       43,
		Inventory: inventory,
	})

	if inventory.SlotLimit != 9 {
		t.Fatalf("pet inventory SlotLimit = %d, want 9", inventory.SlotLimit)
	}
	if inventory.WeightLimit != 109020 {
		t.Fatalf("pet inventory WeightLimit = %d, want %d", inventory.WeightLimit, 109020)
	}
	if got := pet.ScaledExpGain(1000); got != 1750 {
		t.Fatalf("ScaledExpGain(ordinary pet) = %d, want 1750", got)
	}
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
	ids := &sequentialIDs{next: 900}
	gcl := &GameClientLink{world: state, ids: ids, items: store, petItems: petitem.NewService(ids)}
	updates := wireInventoryUpdates(gcl, live)

	gcl.giveItemToPet(context.Background(), live, clientpackets.RequestGiveItemToPet{ObjectID: source.ObjectID, Count: 30})
	updates.Tick()

	if source.Count != 70 {
		t.Fatalf("source Count = %d, want 70", source.Count)
	}
	petStack := petInv.ItemByTemplateID(item.AdenaID)
	if petStack == nil || petStack.Count != 30 || petStack.OwnerID != 0x20000001 || petStack.Location != item.LocationPet {
		t.Fatalf("pet stack = %+v, want 30 adena in pet inventory", petStack)
	}
	// The receiving inventory (the pet's) registers with the batching task
	// first, inside TransferItem's own Add path; the source registers after
	// the transfer completes. The task delivers in registration order.
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodeInventoryUpdate}) {
		t.Fatalf("opcodes = %x, want PetInventoryUpdate then InventoryUpdate", got)
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
	ids := &sequentialIDs{next: 910}
	gcl := &GameClientLink{world: state, ids: ids, items: store, petItems: petitem.NewService(ids)}
	updates := wireInventoryUpdates(gcl, live)

	gcl.getItemFromPet(context.Background(), live, clientpackets.RequestGetItemFromPet{ObjectID: petItem.ObjectID, Count: 15})
	updates.Tick()

	if petItem.Count != 25 {
		t.Fatalf("pet item Count = %d, want 25", petItem.Count)
	}
	playerStack := live.Inventory().ItemByTemplateID(item.AdenaID)
	if playerStack == nil || playerStack.Count != 15 || playerStack.OwnerID != live.ObjectID() || playerStack.Location != item.LocationInventory {
		t.Fatalf("player stack = %+v, want 15 adena in player inventory", playerStack)
	}
	// The receiving inventory (the player's) registers with the batching
	// task first, inside TransferItem's own Add path.
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeInventoryUpdate, serverpackets.OpcodePetInventoryUpdate}) {
		t.Fatalf("opcodes = %x, want InventoryUpdate then PetInventoryUpdate", got)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != petItem.ObjectID || store.updated[0].Count != 25 {
		t.Fatalf("updated rows = %+v, want reduced pet stack", store.updated)
	}
	if len(store.saved) != 1 || store.saved[0].ObjectID != playerStack.ObjectID || store.saved[0].Count != 15 || store.saved[0].OwnerID != live.ObjectID() || store.saved[0].Location != item.LocationInventory {
		t.Fatalf("saved rows = %+v, want new player stack", store.saved)
	}
	_ = petInv
}

func TestPetGetItemPicksUpGroundItem(t *testing.T) {
	templates := petTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, petInv := attachTestPet(t, state, live, templates, 12077, nil)

	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	drops.Drop(ground, task.DropOptions{X: 10, Y: 20, Z: 30})

	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, groundItems: drops, items: store}
	updates := wireInventoryUpdates(gcl, live)

	gcl.petGetItem(context.Background(), live, clientpackets.RequestPetGetItem{ObjectID: ground.ObjectID()})
	updates.Tick()

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodePetInventoryUpdate,
	)
	r := wire.NewReader(capture.frames[0][1:])
	if got := r.ReadInt32(); got != pet.ObjectID() {
		t.Fatalf("GetItem picker id = %d, want pet id %d", got, pet.ObjectID())
	}
	if got := r.ReadInt32(); got != ground.ObjectID() {
		t.Fatalf("GetItem ground id = %d, want %d", got, ground.ObjectID())
	}
	x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if x != 10 || y != 20 || z != 30 {
		t.Fatalf("GetItem location = (%d,%d,%d), want (10,20,30)", x, y, z)
	}
	if _, ok := state.Object(ground.ObjectID()); ok {
		t.Fatalf("world.Object(%d) still present after pickup", ground.ObjectID())
	}
	if got := drops.Len(); got != 0 {
		t.Fatalf("ground item tracker Len = %d, want 0", got)
	}
	petStack := petInv.ItemByTemplateID(item.AdenaID)
	if petStack == nil || petStack.ObjectID != ground.ObjectID() || petStack.Count != 40 || petStack.OwnerID != pet.ObjectID() || petStack.Location != item.LocationPet {
		t.Fatalf("pet stack = %+v, want picked up ground adena", petStack)
	}
	if len(store.saved) != 1 || store.saved[0].ObjectID != ground.ObjectID() || store.saved[0].OwnerID != pet.ObjectID() || store.saved[0].Location != item.LocationPet {
		t.Fatalf("saved rows = %+v, want ground row moved to pet inventory", store.saved)
	}
}

func TestPetGetItemConsumesHerb(t *testing.T) {
	const herbTemplate int32 = 8600
	templates := herbTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, petInv := attachTestPet(t, state, live, templates, 12077, nil)
	tmpl, _ := templates.Get(herbTemplate)
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: herbTemplate, Count: 1, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops := task.NewGroundItems(state, task.GroundItemOptions{HerbAutoDestroy: time.Hour}, time.Now)
	drops.Drop(ground, task.DropOptions{X: 10, Y: 20, Z: 30})

	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{
		world:         state,
		groundItems:   drops,
		items:         store,
		skills:        herbTestSkill(t),
		targets:       skilltarget.NewRegistry(skilltarget.WorldKnown{State: state}),
		skillHandlers: handlerskill.NewDefaultRegistry(),
	}

	gcl.petGetItem(context.Background(), live, clientpackets.RequestPetGetItem{ObjectID: ground.ObjectID()})

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeMagicSkillUse,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeStatusUpdate,
	)
	if petInv.ItemByTemplateID(herbTemplate) != nil || len(store.saved) != 0 || len(store.updated) != 0 {
		t.Fatalf("pet inventory/store retained herb: item=%+v saved=%+v updated=%+v", petInv.ItemByTemplateID(herbTemplate), store.saved, store.updated)
	}
	if effects := pet.EffectList().All(); len(effects) != 1 || effects[0].Skill.ID != 2278 {
		t.Fatalf("pet effects = %+v, want herb skill 2278", effects)
	}
	if effects := live.EffectList().All(); len(effects) != 0 {
		t.Fatalf("owner effects = %+v, want none", effects)
	}
}

func TestPetGetItemReportsNonTradableHerb(t *testing.T) {
	const herbTemplate int32 = 8600
	templates := herbTestTemplates()
	tmpl, _ := templates.Get(herbTemplate)
	tmpl.Tradable = false
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, _ := attachTestPet(t, state, live, templates, 12077, nil)
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: herbTemplate, Count: 1, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops := task.NewGroundItems(state, task.GroundItemOptions{HerbAutoDestroy: time.Hour}, time.Now)
	drops.Drop(ground, task.DropOptions{X: 10, Y: 20, Z: 30})

	capture.frames = nil
	gcl := &GameClientLink{
		world:         state,
		groundItems:   drops,
		skills:        herbTestSkill(t),
		targets:       skilltarget.NewRegistry(skilltarget.WorldKnown{State: state}),
		skillHandlers: handlerskill.NewDefaultRegistry(),
	}

	gcl.petGetItem(context.Background(), live, clientpackets.RequestPetGetItem{ObjectID: ground.ObjectID()})

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeSystemMessage,
	)
	assertStaticSystemMessageFrame(t, capture.frames[2], serverpackets.SystemMessageItemNotForPets)
	if effects := pet.EffectList().All(); len(effects) != 0 {
		t.Fatalf("pet effects = %+v, want none", effects)
	}
}

func TestPetGetItemReportsHerbReuse(t *testing.T) {
	const herbTemplate int32 = 8600
	templates := herbTestTemplates()
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, _ := attachTestPet(t, state, live, templates, 12077, nil)
	pet.DisableSkill(2278*256+1, time.Minute)
	tmpl, _ := templates.Get(herbTemplate)
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: herbTemplate, Count: 1, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops := task.NewGroundItems(state, task.GroundItemOptions{HerbAutoDestroy: time.Hour}, time.Now)
	drops.Drop(ground, task.DropOptions{X: 10, Y: 20, Z: 30})

	capture.frames = nil
	gcl := &GameClientLink{world: state, groundItems: drops, skills: herbTestSkill(t)}

	gcl.petGetItem(context.Background(), live, clientpackets.RequestPetGetItem{ObjectID: ground.ObjectID()})

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeSystemMessage,
	)
	assertSystemMessageSkillFrame(t, capture.frames[2], serverpackets.SystemMessageS1PreparedForReuse, 2278, 1)
}

func TestPetGetItemAcknowledgesUnhandledHerb(t *testing.T) {
	const herbTemplate int32 = 8600
	templates := herbTestTemplates()
	tmpl, _ := templates.Get(herbTemplate)
	tmpl.AttachedSkills = nil
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	_, _ = attachTestPet(t, state, live, templates, 12077, nil)
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: herbTemplate, Count: 1, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops := task.NewGroundItems(state, task.GroundItemOptions{HerbAutoDestroy: time.Hour}, time.Now)
	drops.Drop(ground, task.DropOptions{X: 10, Y: 20, Z: 30})

	capture.frames = nil
	gcl := &GameClientLink{world: state, groundItems: drops, skills: herbTestSkill(t)}

	gcl.petGetItem(context.Background(), live, clientpackets.RequestPetGetItem{ObjectID: ground.ObjectID()})

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodeActionFailed,
	)
}

func TestPetGetItemMergesStackAndDeletesGroundRow(t *testing.T) {
	templates := petTestTemplates()
	petItem := &item.Instance{ObjectID: 901, TemplateID: item.AdenaID, OwnerID: 0x20000001, Count: 10, Location: item.LocationPet}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, petInv := attachTestPet(t, state, live, templates, 12077, []*item.Instance{petItem})

	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}
	drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
	drops.Drop(ground, task.DropOptions{X: 10, Y: 20, Z: 30})

	capture.frames = nil
	store := &recordingEnchantItemStore{}
	gcl := &GameClientLink{world: state, groundItems: drops, items: store}
	updates := wireInventoryUpdates(gcl, live)

	gcl.petGetItem(context.Background(), live, clientpackets.RequestPetGetItem{ObjectID: ground.ObjectID()})
	updates.Tick()

	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeGetItem,
		serverpackets.OpcodeDeleteObject,
		serverpackets.OpcodePetInventoryUpdate,
	)
	petStack := petInv.ItemByTemplateID(item.AdenaID)
	if petStack != petItem || petStack.Count != 50 || petStack.OwnerID != pet.ObjectID() || petStack.Location != item.LocationPet {
		t.Fatalf("pet stack = %+v, want merged 50 adena", petStack)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectID != petItem.ObjectID || store.updated[0].Count != 50 {
		t.Fatalf("updated rows = %+v, want merged pet stack", store.updated)
	}
	if len(store.deleted) != 1 || store.deleted[0] != ground.ObjectID() {
		t.Fatalf("deleted rows = %+v, want absorbed ground row", store.deleted)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved rows = %+v, want none for absorbed ground stack", store.saved)
	}
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
	ids := &sequentialIDs{next: 900}
	gcl := &GameClientLink{world: state, ids: ids, items: store, petItems: petitem.NewService(ids)}
	updates := wireInventoryUpdates(gcl, live)
	gcl.enchantStateStore().Select(live.ObjectID(), scroll.ObjectID)

	gcl.giveItemToPet(context.Background(), live, clientpackets.RequestGiveItemToPet{ObjectID: source.ObjectID, Count: 30})
	updates.Tick()

	if got := gcl.enchantStateStore().Active(live.ObjectID()); got != 0 {
		t.Fatalf("active enchant scroll = %d, want cleared", got)
	}
	assertOpcodeSequence(t, capture.frames,
		serverpackets.OpcodeEnchantResult,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodePetInventoryUpdate,
		serverpackets.OpcodeInventoryUpdate,
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
	updates := wireInventoryUpdates(gcl, live)

	gcl.petUseItem(context.Background(), live, clientpackets.RequestPetUseItem{ObjectID: weapon.ObjectID})
	updates.Tick()

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
	// giveItemToPet's own handler sends nothing on success; sync on a
	// guaranteed-rejected follow-up before driving the tick, so the tick
	// doesn't race the transfer.
	syncBarrier(t, c, func() { c.send(encodeRequestDestroyItem(999999, 1)) }, serverpackets.OpcodeActionFailed)
	inventoryUpdatesFor(t, state).Tick()
	reply := c.read()
	if reply[0] != serverpackets.OpcodePetInventoryUpdate {
		t.Fatalf("first reply opcode = %#x, want PetInventoryUpdate (%#x)", reply[0], serverpackets.OpcodePetInventoryUpdate)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("second reply opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	if stack := petInv.ItemByTemplateID(item.AdenaID); stack == nil || stack.Count != 25 {
		t.Fatalf("pet stack = %+v, want 25 adena", stack)
	}
}

func TestGameClientLinkRequestPetGetItemDispatch(t *testing.T) {
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
	pet, petInv := attachTestPet(t, state, live, testItemTemplates(), 12077, nil)

	c.send(encodeRequestDropItem(500, 40, location.Location{X: 10, Y: 20, Z: 30}))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeDropItem {
		t.Fatalf("drop broadcast opcode = %#x, want DropItem (%#x)", reply[0], serverpackets.OpcodeDropItem)
	}
	r := wire.NewReader(reply[1:])
	r.ReadInt32()
	groundID := r.ReadInt32()

	inventoryUpdatesFor(t, state).Tick()
	if reply := c.read(); reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("drop inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}

	c.send(encodeRequestPetGetItem(groundID))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeGetItem {
		t.Fatalf("pickup opcode = %#x, want GetItem (%#x)", reply[0], serverpackets.OpcodeGetItem)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != pet.ObjectID() {
		t.Fatalf("GetItem picker id = %d, want pet id %d", got, pet.ObjectID())
	}
	if got := r.ReadInt32(); got != groundID {
		t.Fatalf("GetItem ground id = %d, want %d", got, groundID)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("pickup delete opcode = %#x, want DeleteObject (%#x)", reply[0], serverpackets.OpcodeDeleteObject)
	}
	inventoryUpdatesFor(t, state).Tick()
	reply = c.read()
	if reply[0] != serverpackets.OpcodePetInventoryUpdate {
		t.Fatalf("pickup inventory opcode = %#x, want PetInventoryUpdate (%#x)", reply[0], serverpackets.OpcodePetInventoryUpdate)
	}
	if _, ok := state.Object(groundID); ok {
		t.Fatalf("world.Object(%d) still present after pickup", groundID)
	}
	if stack := petInv.ItemByTemplateID(item.AdenaID); stack == nil || stack.Count != 40 || stack.OwnerID != pet.ObjectID() {
		t.Fatalf("pet stack = %+v, want 40 adena", stack)
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

	gcl.handleTargetAction(context.Background(), live, pet.ObjectID(), false, false)
	capture.frames = nil
	gcl.handleTargetAction(context.Background(), live, pet.ObjectID(), true, false)

	// Interacting with an owned summon must also release the pending action
	// the client registered for the click, or its input stays locked.
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodePetStatusShow}) {
		t.Fatalf("opcodes = %x, want ActionFailed, PetStatusShow", got)
	}
	r := wire.NewReader(capture.frames[1][1:])
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

func encodeRequestPetGetItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPetGetItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}
