package network

import (
	"context"
	"testing"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// petConsumableTestTemplates carries one pet potion (ItemSkills-handled,
// carrying a healing-potion skill) and one pet food (PetFoods-handled),
// matching real datapack entries (item 1060, item 2515).
func petConsumableTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:             1060,
			Name:           "Lesser Healing Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills"},
			AttachedSkills: []item.SkillRef{{ID: 2031, Level: 1}},
		},
		{
			ID:          2515,
			Name:        "Wolf's food",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Handler: "PetFoods"},
		},
	})
}

func petConsumableTestSkill(t *testing.T) *skillstate.Persistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 5, Time: 3, Value: 12, Icon: true}},
		},
		{
			ID: 2048, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "FEED", Feed: 40,
		},
	}), store)
}

// TestPetUseItemConsumesPotion is the regression test for #1582: a pet
// potion (etc-item handler "ItemSkills") reaching RequestPetUseItem must
// dispatch to the item's carried skill instead of falling through to
// PET_CANNOT_USE_ITEM, matching RequestPetUseItem.java's else branch.
func TestPetUseItemConsumesPotion(t *testing.T) {
	templates := petConsumableTestTemplates()
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	pet, petInv := attachTestPet(t, state, live, templates, 12077, []*item.Instance{
		{ObjectID: 900, TemplateID: 1060, Count: 1, Location: item.LocationPet},
	})

	testsupport.ResetCapture(capture)
	gcl := &GameClientLink{
		world:         state,
		skills:        petConsumableTestSkill(t),
		targets:       skilltarget.NewRegistry(skilltarget.WorldKnown{State: state}),
		skillHandlers: handlerskill.NewDefaultRegistry(),
	}

	gcl.petUseItem(context.Background(), live, clientpackets.RequestPetUseItem{ObjectID: 900})

	testsupport.AssertOpcodeSequence(t, capture.Frames(),
		serverpackets.OpcodeMagicSkillUse,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeStatusUpdate,
	)
	assertSystemMessageSkillFrame(t, capture.Frames()[1], serverpackets.SystemMessagePetUsesS1, 2031, 1)
	if petInv.ItemByObjectID(900) != nil {
		t.Fatalf("pet inventory retained consumed potion")
	}
	if effects := pet.EffectList().All(); len(effects) != 1 || effects[0].Skill.ID != 2031 {
		t.Fatalf("pet effects = %+v, want potion skill 2031", effects)
	}
}

// TestPetUseItemConsumesFood is the regression test for #1582's other
// branch: pet food (etc-item handler "PetFoods") matching the pet's
// template food id must be eaten (destroyed, meal gauge raised) rather
// than rejected.
func TestPetUseItemConsumesFood(t *testing.T) {
	templates := petConsumableTestTemplates()
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	petInv := itemcontainer.NewPetInventory(0x20000001, templates)
	petInv.Restore([]*item.Instance{
		{ObjectID: 900, TemplateID: 2515, Count: 1, Location: item.LocationPet},
	})
	pet := summon.NewPet(summon.PetConfig{
		ObjectID:      0x20000001,
		Owner:         live,
		NPCID:         12077,
		Level:         1,
		Inventory:     petInv,
		Fed:           10,
		MaxMeal:       100,
		Food1:         2515,
		AutoFeedLimit: 0.55,
	})
	summon.SpawnBesideOwner(state, pet, live, location.Location{X: 10})

	testsupport.ResetCapture(capture)
	gcl := &GameClientLink{
		world:         state,
		skills:        petConsumableTestSkill(t),
		targets:       skilltarget.NewRegistry(skilltarget.WorldKnown{State: state}),
		skillHandlers: handlerskill.NewDefaultRegistry(),
	}
	gcl.petConfig.FoodRate = 1

	gcl.petUseItem(context.Background(), live, clientpackets.RequestPetUseItem{ObjectID: 900})

	testsupport.AssertOpcodeSequence(t, capture.Frames(),
		serverpackets.OpcodeMagicSkillUse,
		serverpackets.OpcodeSystemMessage,
	)
	assertStaticSystemMessageFrame(t, capture.Frames()[1], serverpackets.SystemMessageYourPetAteALittleButIsStillHungry)
	if petInv.ItemByObjectID(900) != nil {
		t.Fatalf("pet inventory retained eaten food")
	}
	if fed := pet.Fed(); fed != 50 {
		t.Fatalf("fed = %d, want 50 (10 + Feed 40)", fed)
	}
}

// TestGameSummonSpawnerWiresAutoFeedRestore is the regression test for
// #1730: FoodRestore was never set at spawn, so TickPet's auto-feed branch
// (live_lifecycle.go:130) always added 0 to the meal gauge. It must be
// wired from the same feed skill PetFoods.java uses for the manual "eat
// from inventory" path (#1582), since both dispatch through the same item
// handler in the reference (PetFoods.java:50-70).
func TestGameSummonSpawnerWiresAutoFeedRestore(t *testing.T) {
	templates := petConsumableTestTemplates()
	npcs := npc.NewTable([]*npc.Template{{
		ID: 12077, Name: "Wolf", Level: 10,
		Pet: &npc.PetData{
			Food1:         2515,
			AutoFeedLimit: 0.55,
			Levels: map[int]npc.PetLevelStats{
				10: {MaxHP: 400, MaxMP: 80, MaxMeal: 100, MealInNormal: 0, MealInBattle: 0},
			},
		},
	}})
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: summonTestCollarTemplateID, NPCID: 12077, SummonType: summonItemTypePet},
	})
	if err != nil {
		t.Fatalf("build summon item table: %v", err)
	}
	state := world.New()
	link := NewGameClientLink(GameClientLinkConfig{
		World:         state,
		AI:            task.NewAI(state),
		NPCs:          npcs,
		SummonItems:   summonItems,
		PetStore:      fakePetStoreNoSaved{},
		IDs:           &fakeSummonIDs{},
		Skills:        petConsumableTestSkill(t),
		ItemTemplates: templates,
	})
	link.petConfig.FoodRate = 1
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	state.Spawn(live, 0, 0, 0, 0)
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}

	if !(&gameSummonSpawner{link: link, live: live}).SpawnPet(live.Character, inst) {
		t.Fatal("SpawnPet returned false")
	}
	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatal("pet not registered in world.State")
	}
	pet := obj.(*summon.Actor)
	pet.PetInventory().Restore([]*item.Instance{
		{ObjectID: 900, TemplateID: 2515, Count: 1, Location: item.LocationPet},
	})

	if fed, _ := pet.AddFed(-90); fed != 10 {
		t.Fatalf("Fed() after AddFed(-90) = %d, want 10", fed)
	}

	result := pet.TickPet(state)
	if !result.AutoFed {
		t.Fatalf("TickPet result = %+v, want AutoFed", result)
	}
	if fed := pet.Fed(); fed != 50 {
		t.Fatalf("Fed() after auto-feed = %d, want 50 (10 + Feed 40)", fed)
	}
	if pet.PetInventory().ItemByObjectID(900) != nil {
		t.Fatalf("pet inventory retained auto-fed food")
	}
}

// TestPetUseItemRejectsIneligibleConsumable pins the existing rejection
// path: a non-equipment item that is neither a potion nor food the pet's
// template accepts still answers PET_CANNOT_USE_ITEM.
func TestPetUseItemRejectsIneligibleConsumable(t *testing.T) {
	templates := item.NewTable([]*item.Template{{
		ID:          57,
		Name:        "Adena",
		Kind:        item.KindEtcItem,
		Duration:    -1,
		Stackable:   true,
		Dropable:    true,
		Tradable:    true,
		Destroyable: true,
		EtcItem:     &item.EtcItemDetail{},
	}})
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, templates, nil)
	state := world.New()
	state.Spawn(live, 0, 0, 0, 0)
	_, petInv := attachTestPet(t, state, live, templates, 12077, []*item.Instance{
		{ObjectID: 900, TemplateID: 57, Count: 1, Location: item.LocationPet},
	})

	testsupport.ResetCapture(capture)
	gcl := &GameClientLink{world: state}

	gcl.petUseItem(context.Background(), live, clientpackets.RequestPetUseItem{ObjectID: 900})

	testsupport.AssertOpcodeSequence(t, capture.Frames(), serverpackets.OpcodeSystemMessage)
	assertStaticSystemMessageFrame(t, capture.Frames()[0], serverpackets.SystemMessagePetCannotUseItem)
	if petInv.ItemByObjectID(900) == nil {
		t.Fatalf("pet inventory lost rejected item")
	}
}
