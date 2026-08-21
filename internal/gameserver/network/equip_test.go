package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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
		CharLevel: 1,
		Location:  location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	ch.SetResourceValues(player.Resources{MaxHP: 80, CurrentHP: 80, MaxMP: 30, CurrentMP: 30})
	ch.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(ch.ID, templates, items))
	ch.SetFrameSender(capture.send)
	ch.SetBroadcastFrameSender(capture.send)

	live, err := creature.NewLive(ch.Location, tmpl.RunSpeed, testGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live

	return &livePlayer{Character: ch, template: tmpl, items: items, visibilitySend: capture.send}
}

// wireInventoryUpdates gives gcl a batching task and registers live's
// inventory with it, the way character_flow.go's spawn wiring does for a
// live player built through the full login flow. Tests that construct
// *GameClientLink and *livePlayer directly need this to exercise
// InventoryUpdate delivery, now that the task is the packet's only sender.
// It also spawns live into a fresh world.State if it isn't already visible
// somewhere: the task's tick gate skips an owner that isn't visible or
// teleporting, and a live player built directly rather than through the
// full login flow starts out in no world at all.
//
// If live already has a spawned pet (attachTestPet ran first, as in the pet
// tests), this also registers the pet's inventory — the structural
// attach-point wiring newPet does in production, done here once for tests
// that build the pet directly rather than through newPet.
func wireInventoryUpdates(gcl *GameClientLink, live *livePlayer) *task.InventoryUpdates {
	updates := task.NewInventoryUpdates()
	gcl.inventoryUpdates = updates
	if inv := live.Inventory(); inv != nil {
		inv.SetUpdateNotifier(func() {
			updates.Add(inv, live)
		})
	}
	if !live.Visible() {
		world.New().Spawn(live, 0, 0, 0, 0)
	}
	if gcl.world != nil {
		if obj, ok := gcl.world.Summon(live.ObjectID()); ok {
			if pet, ok := obj.(*summon.Actor); ok {
				gcl.registerPetInventoryUpdates(pet, live)
			}
		}
	}
	return updates
}

// equipFleeTarget satisfies the flee hook a Fear effect's runtime needs, so
// it activates regardless of what its actual effected actor is.
type equipFleeTarget struct{}

func (equipFleeTarget) ObjectID() int32                                    { return 0 }
func (equipFleeTarget) Dead() bool                                         { return false }
func (equipFleeTarget) FleeFrom(effector effect.Participant, distance int) {}

// addEquipTestEffect installs an active core effect of name on live, for
// exercising crowd-control gating without wiring a full skill cast.
func addEquipTestEffect(t *testing.T, live *livePlayer, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = equipFleeTarget{}
	live.EffectList().Add(e)
	return e
}

func TestUseItemTogglesEquipState(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 10, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}}})
	weapon := &item.Instance{ObjectID: 500, TemplateID: 10, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
	gcl := &GameClientLink{}
	updates := wireInventoryUpdates(gcl, live)

	gcl.useItem(live, weapon.ObjectID)
	updates.Tick()

	if !weapon.Equipped() {
		t.Fatal("weapon not equipped after first UseItem")
	}
	if weapon.Location != item.LocationPaperdoll || weapon.LocationData != itemcontainer.RHand {
		t.Fatalf("weapon location = %v/%d, want paperdoll/RHand", weapon.Location, weapon.LocationData)
	}
	if len(capture.frames) != 2 || capture.frames[0][0] != serverpackets.OpcodeUserInfo || capture.frames[1][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("frames after equip = %x, want UserInfo then InventoryUpdate", capture.frames)
	}
	capture.frames = nil

	gcl.useItem(live, weapon.ObjectID)
	updates.Tick()

	if weapon.Equipped() {
		t.Fatal("weapon still equipped after second UseItem")
	}
	if weapon.Location != item.LocationInventory {
		t.Fatalf("weapon location = %v, want inventory", weapon.Location)
	}
	if len(capture.frames) != 2 || capture.frames[0][0] != serverpackets.OpcodeUserInfo || capture.frames[1][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("frames after unequip = %x, want UserInfo then InventoryUpdate", capture.frames)
	}
}

func TestUseItemAttachesAndUnequipDetachesItemStats(t *testing.T) {
	templates := item.NewTable([]*item.Template{{
		ID: 10, Kind: item.KindWeapon, Slot: item.SlotRHand,
		Weapon:    &item.WeaponDetail{Type: item.WeaponSword},
		Modifiers: []item.StatModifier{{Op: item.FuncAdd, Stat: "mAtk", Value: 17}},
	}})
	weapon := &item.Instance{ObjectID: 500, TemplateID: 10, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
	gcl := &GameClientLink{skills: skillstate.NewPersistence(nil, modelskill.NewTable(nil))}
	baseMAtk := live.MAtk()

	gcl.useItem(live, weapon.ObjectID)

	if got, want := live.MAtk(), baseMAtk+17; got != want {
		t.Fatalf("MAtk() after equip = %v, want %v", got, want)
	}

	gcl.useItem(live, weapon.ObjectID)

	if got := live.MAtk(); got != baseMAtk {
		t.Fatalf("MAtk() after unequip = %v, want unchanged %v", got, baseMAtk)
	}
}

// TestUseItemGrantsNonPassiveAttachedSkillAndSendsSkillListAndCoolTime pins
// issue 1398: equipping an item whose AttachedSkills entry is an ACTIVE
// (non-passive) item_skill must grant it and reach the client via SkillList
// and, since it has a positive equip delay, SkillCoolTime — mirroring
// ItemPassiveSkillsListener.onEquip. Unequipping must resend SkillList only.
func TestUseItemGrantsNonPassiveAttachedSkillAndSendsSkillListAndCoolTime(t *testing.T) {
	const itemSkillID = 3264
	templates := item.NewTable([]*item.Template{{
		ID: 9184, Kind: item.KindArmor, Slot: item.SlotHairAll,
		Armor:          &item.ArmorDetail{Type: item.ArmorLight},
		AttachedSkills: []item.SkillRef{{ID: itemSkillID, Level: 1}},
	}})
	hat := &item.Instance{ObjectID: 500, TemplateID: 9184, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{hat})
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: itemSkillID, Level: 1, Activation: modelskill.ActivationActive, EquipDelay: 30000},
	})
	gcl := &GameClientLink{skills: skillstate.NewPersistence(nil, skills)}

	gcl.useItem(live, hat.ObjectID)

	if got := live.SkillLevel(itemSkillID); got != 1 {
		t.Fatalf("SkillLevel(%d) after equip = %d, want 1", itemSkillID, got)
	}
	var sawSkillList, sawCoolTime bool
	for _, f := range capture.frames {
		switch f[0] {
		case serverpackets.OpcodeSkillList:
			sawSkillList = true
		case serverpackets.OpcodeSkillCoolTime:
			sawCoolTime = true
		}
	}
	if !sawSkillList {
		t.Fatalf("frames after equip = %x, want a SkillList frame", capture.frames)
	}
	if !sawCoolTime {
		t.Fatalf("frames after equip = %x, want a SkillCoolTime frame (equip delay armed)", capture.frames)
	}
	capture.frames = nil

	gcl.useItem(live, hat.ObjectID)

	if got := live.SkillLevel(itemSkillID); got != 0 {
		t.Fatalf("SkillLevel(%d) after unequip = %d, want 0 (revoked)", itemSkillID, got)
	}
	sawSkillList, sawCoolTime = false, false
	for _, f := range capture.frames {
		switch f[0] {
		case serverpackets.OpcodeSkillList:
			sawSkillList = true
		case serverpackets.OpcodeSkillCoolTime:
			sawCoolTime = true
		}
	}
	if !sawSkillList {
		t.Fatalf("frames after unequip = %x, want a SkillList frame", capture.frames)
	}
	if sawCoolTime {
		t.Fatalf("frames after unequip = %x, want no SkillCoolTime frame (reference sends none on unequip)", capture.frames)
	}
}

func TestUseItemUnknownObjectIDIsNoop(t *testing.T) {
	templates := item.NewTable(nil)
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	gcl := &GameClientLink{}

	gcl.useItem(live, 999)

	// ActionFailed must still answer a rejected use: the client's item
	// window locks the clicked slot waiting for a response.
	if len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("frames for unknown object id = %x, want ActionFailed only", capture.frames)
	}
}

func TestUnequipItemBySlot(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}}})
	chest := &item.Instance{ObjectID: 501, TemplateID: 20, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{chest})
	gcl := &GameClientLink{}
	updates := wireInventoryUpdates(gcl, live)

	gcl.unequipItem(live, int32(item.SlotChest))
	updates.Tick()

	if chest.Equipped() {
		t.Fatal("chest piece still equipped after RequestUnEquipItem")
	}
	if len(capture.frames) != 3 || capture.frames[0][0] != serverpackets.OpcodeUserInfo || capture.frames[1][0] != serverpackets.OpcodeSystemMessage || capture.frames[2][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("frames after unequip = %x, want UserInfo, S1_DISARMED SystemMessage, then InventoryUpdate", capture.frames)
	}
	if r := wire.NewReader(capture.frames[1][1:]); r.ReadInt32() != serverpackets.SystemMessageS1Disarmed {
		t.Fatalf("unenchanted unequip message id != S1_DISARMED")
	}
}

// TestUnequipItemEnchantedSendsEquipmentRemoved pins RequestUnEquipItem.java's
// success branch on an enchanted item: EQUIPMENT_S1_S2_REMOVED (enchant
// level + item name), not S1_DISARMED.
func TestUnequipItemEnchantedSendsEquipmentRemoved(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}}})
	chest := &item.Instance{ObjectID: 501, TemplateID: 20, EnchantLevel: 6, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{chest})
	gcl := &GameClientLink{}
	updates := wireInventoryUpdates(gcl, live)

	gcl.unequipItem(live, int32(item.SlotChest))
	updates.Tick()

	if len(capture.frames) != 3 || capture.frames[1][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("frames after enchanted unequip = %x, want UserInfo, EQUIPMENT_S1_S2_REMOVED SystemMessage, then InventoryUpdate", capture.frames)
	}
	r := wire.NewReader(capture.frames[1][1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageEquipmentS1S2Removed {
		t.Fatalf("enchanted unequip message id = %d, want EQUIPMENT_S1_S2_REMOVED", id)
	}
}

// TestUnequipItemDuringCastIsRejected pins RequestUnEquipItem.java:37's
// casting gate: unequip while mid-cast is rejected with S1_CANNOT_BE_USED,
// the same as the other crowd-control states, instead of silently applying.
func TestUnequipItemDuringCastIsRejected(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}}})
	chest := &item.Instance{ObjectID: 501, TemplateID: 20, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{chest})
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	capture.frames = nil

	gcl := &GameClientLink{world: state, geo: testGeo{}}
	controller := gcl.castController(live)
	if _, err := controller.Start(time.Now(), live, castingDef); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	gcl.unequipItem(live, int32(item.SlotChest))

	if !chest.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("mid-cast RequestUnEquipItem mutated item=%+v frames=%x, want unchanged item and S1_CANNOT_BE_USED SystemMessage only", chest, capture.frames)
	}
	r := wire.NewReader(capture.frames[0][1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageS1CannotBeUsed {
		t.Fatalf("mid-cast unequip message id = %d, want S1_CANNOT_BE_USED", id)
	}
}

func TestUnequipItemEmptySlotIsNoop(t *testing.T) {
	templates := item.NewTable(nil)
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	gcl := &GameClientLink{}

	gcl.unequipItem(live, int32(item.SlotChest))

	if len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("frames for empty slot = %x, want ActionFailed only", capture.frames)
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
	updates := wireInventoryUpdates(gcl, wearer)
	gcl.useItem(wearer, weapon.ObjectID)
	updates.Tick()

	if len(wearerFrames.frames) != 2 || wearerFrames.frames[0][0] != serverpackets.OpcodeUserInfo || wearerFrames.frames[1][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("wearer frames = %x, want UserInfo then InventoryUpdate", wearerFrames.frames)
	}
	if len(observerFrames.frames) != 1 || observerFrames.frames[0][0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("observer frames = %x, want one CharInfo", observerFrames.frames)
	}
}

func TestDeadPlayerItemOpsAreNoops(t *testing.T) {
	t.Run("use item", func(t *testing.T) {
		templates := testItemTemplates()
		weapon := &item.Instance{ObjectID: 500, TemplateID: 30, Location: item.LocationInventory}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
		live.MarkDead()

		(&GameClientLink{}).useItem(live, weapon.ObjectID)

		if weapon.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("dead UseItem mutated item=%+v frames=%x, want unchanged item and ActionFailed only", weapon, capture.frames)
		}
	})

	t.Run("unequip item", func(t *testing.T) {
		templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}}})
		chest := &item.Instance{ObjectID: 501, TemplateID: 20, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{chest})
		live.MarkDead()

		(&GameClientLink{}).unequipItem(live, int32(item.SlotChest))

		if !chest.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeSystemMessage {
			t.Fatalf("dead RequestUnEquipItem mutated item=%+v frames=%x, want unchanged item and S1_CANNOT_BE_USED SystemMessage only", chest, capture.frames)
		}
	})

	t.Run("destroy item", func(t *testing.T) {
		templates := testItemTemplates()
		stack := &item.Instance{ObjectID: 502, TemplateID: 20, Count: 5, Location: item.LocationInventory}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{stack})
		live.MarkDead()

		(&GameClientLink{}).destroyLiveItem(live, stack.ObjectID, 2)

		if stack.Count != 5 || len(capture.frames) != 0 {
			t.Fatalf("dead RequestDestroyItem mutated item=%+v frames=%x, want unchanged item and no frames", stack, capture.frames)
		}
	})

	t.Run("crystallize item", func(t *testing.T) {
		templates := testItemTemplates()
		weapon := &item.Instance{ObjectID: 503, TemplateID: 30, Count: 1, Location: item.LocationInventory}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
		live.SetSkillLevel(248, 1)
		live.MarkDead()

		(&GameClientLink{ids: &sequentialIDs{next: 100}}).crystallizeLiveItem(live, clientpackets.RequestCrystallizeItem{ObjectID: weapon.ObjectID, Count: 1})

		if live.Inventory().ItemByObjectID(weapon.ObjectID) == nil || len(capture.frames) != 0 {
			t.Fatalf("dead RequestCrystallizeItem mutated inventory frames=%x, want unchanged inventory and no frames", capture.frames)
		}
	})
}

func TestItemOperationRejectMessages(t *testing.T) {
	t.Run("drop zero count", func(t *testing.T) {
		templates := item.NewTable([]*item.Template{{ID: item.AdenaID, Kind: item.KindEtcItem, Duration: -1, Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{}}})
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{{ObjectID: 500, TemplateID: item.AdenaID, Count: 1, Location: item.LocationInventory}})

		(&GameClientLink{groundItems: &recordingGroundDropper{}}).dropLiveItem(live, clientpackets.RequestDropItem{ObjectID: 500})

		if len(capture.frames) != 1 {
			t.Fatalf("frames = %x, want one CannotDiscardThisItem", capture.frames)
		}
		assertStaticSystemMessageFrame(t, capture.frames[0], 98)
	})

	t.Run("destroy invalid count", func(t *testing.T) {
		templates := item.NewTable([]*item.Template{{ID: item.AdenaID, Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{}}})
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{{ObjectID: 500, TemplateID: item.AdenaID, Count: 1, Location: item.LocationInventory}})

		(&GameClientLink{}).destroyLiveItem(live, 500, 0)

		if len(capture.frames) != 1 {
			t.Fatalf("frames = %x, want one CannotDestroyNumberIncorrect", capture.frames)
		}
		assertStaticSystemMessageFrame(t, capture.frames[0], 163)
	})

	t.Run("destroy hero item", func(t *testing.T) {
		templates := item.NewTable([]*item.Template{{ID: 6611, Kind: item.KindWeapon, Duration: -1, Destroyable: true, Weapon: &item.WeaponDetail{}}})
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{{ObjectID: 500, TemplateID: 6611, Count: 1, Location: item.LocationInventory}})

		(&GameClientLink{}).destroyLiveItem(live, 500, 1)

		if len(capture.frames) != 1 {
			t.Fatalf("frames = %x, want one HeroWeaponsCantDestroyed", capture.frames)
		}
		assertStaticSystemMessageFrame(t, capture.frames[0], 1845)
	})
}

func TestCrowdControlledPlayerItemOpsAreNoops(t *testing.T) {
	effectNames := []string{"Stun", "Sleep", "Paralyze", "Fear"}

	for _, effectName := range effectNames {
		t.Run(effectName+"/use item", func(t *testing.T) {
			templates := testItemTemplates()
			weapon := &item.Instance{ObjectID: 500, TemplateID: 30, Location: item.LocationInventory}
			capture := &frameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
			addEquipTestEffect(t, live, effectName)

			(&GameClientLink{}).useItem(live, weapon.ObjectID)

			if weapon.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("%s UseItem mutated item=%+v frames=%x, want unchanged item and ActionFailed only", effectName, weapon, capture.frames)
			}
		})

		t.Run(effectName+"/unequip item", func(t *testing.T) {
			templates := item.NewTable([]*item.Template{{ID: 20, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{Type: item.ArmorLight}}})
			chest := &item.Instance{ObjectID: 501, TemplateID: 20, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest}
			capture := &frameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{chest})
			addEquipTestEffect(t, live, effectName)

			(&GameClientLink{}).unequipItem(live, int32(item.SlotChest))

			if !chest.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeSystemMessage {
				t.Fatalf("%s RequestUnEquipItem mutated item=%+v frames=%x, want unchanged item and S1_CANNOT_BE_USED SystemMessage only", effectName, chest, capture.frames)
			}
		})
	}

	t.Run("manual paralysis lock/use item", func(t *testing.T) {
		templates := testItemTemplates()
		weapon := &item.Instance{ObjectID: 500, TemplateID: 30, Location: item.LocationInventory}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{weapon})
		live.SetParalyzed(true)

		(&GameClientLink{}).useItem(live, weapon.ObjectID)

		if weapon.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("paralyzed UseItem mutated item=%+v frames=%x, want unchanged item and ActionFailed only", weapon, capture.frames)
		}
	})
}

// TestCrowdControlledPlayerCanStillDropDestroyAndPickUp is the other half
// of TestCrowdControlledPlayerItemOpsAreNoops: RequestDropItem.java:36 gates
// dead-only and RequestDestroyItem gates nothing, so the same crowd-control
// effects that block use/unequip must leave drop and destroy reachable.
// Pickup is the exception, not a third example of the pattern: it routes
// through PlayableAI.tryToPickUp's denyAiAction() gate (PlayableAI.java:
// 411-417, Creature.java:636-639) before the flying-only check in
// thinkPickUp ever runs, so it's CC-gated the same as use/unequip — see the
// separate pickup subtest below.
func TestCrowdControlledPlayerCanStillDropDestroyAndPickUp(t *testing.T) {
	effectNames := []string{"Stun", "Sleep", "Paralyze", "Fear"}

	for _, effectName := range effectNames {
		t.Run(effectName+"/drop", func(t *testing.T) {
			templates := testItemTemplates()
			stack := &item.Instance{ObjectID: 500, TemplateID: item.AdenaID, Count: 100, Location: item.LocationInventory}
			capture := &frameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{stack})
			addEquipTestEffect(t, live, effectName)
			drops := &recordingGroundDropper{}

			(&GameClientLink{ids: &sequentialIDs{next: 200}, groundItems: drops, inventory: invops.NewService(&sequentialIDs{next: 300})}).dropLiveItem(live, clientpackets.RequestDropItem{
				ObjectID: stack.ObjectID,
				Count:    40,
				X:        110,
				Y:        0,
				Z:        0,
			})

			if stack.Count != 60 || len(drops.drops) != 1 {
				t.Fatalf("%s drop mutated count=%d drops=%d, want count 60 and 1 drop", effectName, stack.Count, len(drops.drops))
			}
			if len(capture.frames) != 0 {
				t.Fatalf("%s drop frames=%x, want none (no rejection)", effectName, capture.frames)
			}
		})

		t.Run(effectName+"/destroy", func(t *testing.T) {
			templates := testItemTemplates()
			stack := &item.Instance{ObjectID: 500, TemplateID: item.AdenaID, Count: 5, Location: item.LocationInventory}
			capture := &frameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{stack})
			addEquipTestEffect(t, live, effectName)

			(&GameClientLink{inventory: invops.NewService(&sequentialIDs{next: 300})}).destroyLiveItem(live, stack.ObjectID, 2)

			if stack.Count != 3 {
				t.Fatalf("%s destroy mutated count=%d, want 3", effectName, stack.Count)
			}
			if len(capture.frames) != 0 {
				t.Fatalf("%s destroy frames=%x, want none (no rejection)", effectName, capture.frames)
			}
		})

	}

	t.Run("manual paralysis lock/drop", func(t *testing.T) {
		templates := testItemTemplates()
		stack := &item.Instance{ObjectID: 500, TemplateID: item.AdenaID, Count: 100, Location: item.LocationInventory}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{stack})
		live.SetParalyzed(true)
		drops := &recordingGroundDropper{}

		(&GameClientLink{ids: &sequentialIDs{next: 200}, groundItems: drops, inventory: invops.NewService(&sequentialIDs{next: 300})}).dropLiveItem(live, clientpackets.RequestDropItem{
			ObjectID: stack.ObjectID,
			Count:    40,
			X:        110,
			Y:        0,
			Z:        0,
		})

		if stack.Count != 60 || len(drops.drops) != 1 || len(capture.frames) != 0 {
			t.Fatalf("paralyzed drop mutated count=%d drops=%d frames=%x, want count 60, 1 drop, no frames", stack.Count, len(drops.drops), capture.frames)
		}
	})
}

// TestCrowdControlledPlayerCannotPickUp mirrors
// TestCrowdControlledPlayerItemOpsAreNoops: pickup routes through
// PlayableAI.tryToPickUp's denyAiAction() gate (PlayableAI.java:411-417,
// Creature.java:636-639) before the flying-only check in thinkPickUp ever
// runs, so — unlike drop/destroy — it's CC-gated the same as use/unequip.
func TestCrowdControlledPlayerCannotPickUp(t *testing.T) {
	effectNames := []string{"Stun", "Sleep", "Paralyze", "Fear"}

	for _, effectName := range effectNames {
		t.Run(effectName, func(t *testing.T) {
			templates := petTestTemplates()
			capture := &frameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
			state := world.New()
			state.Spawn(live, 100, 0, 0, 0)
			drops := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Hour}, time.Now)
			tmpl, _ := templates.Get(item.AdenaID)
			ground := dropTestGround(t, state, drops, item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl, 100, 0, 0)
			addEquipTestEffect(t, live, effectName)
			store := &recordingEnchantItemStore{}
			capture.frames = nil // drop the setup-time SpawnItem broadcast

			gcl := &GameClientLink{world: state, groundItems: drops, items: store}
			if !gcl.pickupLiveGroundItem(context.Background(), live, ground) {
				t.Fatal(effectName + " pickupLiveGroundItem returned false for a ground item target")
			}

			if stack := live.Inventory().ItemByTemplateID(item.AdenaID); stack != nil {
				t.Fatalf("%s pickup mutated inventory = %+v, want no pickup", effectName, stack)
			}
			if got := drops.Len(); got != 1 {
				t.Fatalf("%s pickup ground item tracker Len = %d, want 1 (item still on ground)", effectName, got)
			}
			if len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("%s pickup frames=%x, want ActionFailed only", effectName, capture.frames)
			}
		})
	}
}

func TestDropLiveItemRejectsFarCoordinatesBeforeInventoryMutation(t *testing.T) {
	templates := testItemTemplates()
	stack := &item.Instance{ObjectID: 504, TemplateID: item.AdenaID, Count: 100, Location: item.LocationInventory}
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{stack})
	drops := &recordingGroundDropper{}

	(&GameClientLink{ids: &sequentialIDs{next: 200}, groundItems: drops}).dropLiveItem(live, clientpackets.RequestDropItem{
		ObjectID: stack.ObjectID,
		Count:    40,
		X:        10000,
		Y:        0,
		Z:        0,
	})

	if stack.Count != 100 || len(drops.drops) != 0 {
		t.Fatalf("far drop mutated count=%d drops=%d", stack.Count, len(drops.drops))
	}
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("far drop opcodes = %x, want SystemMessage only", got)
	}
	r := wire.NewReader(capture.frames[0][1:])
	if id := r.ReadInt32(); id != 151 {
		t.Fatalf("far drop system message id = %d, want 151", id)
	}
}
