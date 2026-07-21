package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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

	live, err := creature.NewLive(ch.Location, tmpl.RunSpeed, testGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live

	return &livePlayer{Character: ch, template: tmpl, items: items}
}

// equipFleeTarget satisfies the flee hook a Fear effect's runtime needs, so
// it activates regardless of what its actual effected actor is.
type equipFleeTarget struct{}

func (equipFleeTarget) FleeFrom(effector any, distance int) {}

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
	gcl.useItem(wearer, weapon.ObjectID)

	if len(wearerFrames.frames) != 2 || wearerFrames.frames[0][0] != serverpackets.OpcodeInventoryUpdate || wearerFrames.frames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("wearer frames = %x, want InventoryUpdate then UserInfo", wearerFrames.frames)
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

		if !chest.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("dead RequestUnEquipItem mutated item=%+v frames=%x, want unchanged item and ActionFailed only", chest, capture.frames)
		}
	})

	t.Run("destroy item", func(t *testing.T) {
		templates := testItemTemplates()
		stack := &item.Instance{ObjectID: 502, TemplateID: 20, Count: 5, Location: item.LocationInventory}
		capture := &frameCapture{}
		live := newEquipTestLivePlayer(t, 1, capture, templates, []*item.Instance{stack})
		live.MarkDead()

		(&GameClientLink{}).destroyLiveItem(live, stack.ObjectID, 2)

		if stack.Count != 5 || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("dead RequestDestroyItem mutated item=%+v frames=%x, want unchanged item and ActionFailed only", stack, capture.frames)
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

		if live.Inventory().ItemByObjectID(weapon.ObjectID) == nil || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("dead RequestCrystallizeItem mutated inventory frames=%x, want unchanged inventory and ActionFailed only", capture.frames)
		}
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

			if !chest.Equipped() || len(capture.frames) != 1 || capture.frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("%s RequestUnEquipItem mutated item=%+v frames=%x, want unchanged item and ActionFailed only", effectName, chest, capture.frames)
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
	if got := frameOpcodes(capture.frames); string(got) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("far drop opcodes = %x, want SystemMessage, ActionFailed", got)
	}
	r := wire.NewReader(capture.frames[0][1:])
	if id := r.ReadInt32(); id != 151 {
		t.Fatalf("far drop system message id = %d, want 151", id)
	}
}
