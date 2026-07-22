package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// consumableSkillTable seeds the two instant-cast potion skills the item
// templates in testItemTemplates reference: a heal-over-time potion and a
// percent-of-max mana potion. Both are flagged as potions so the use-item
// path applies them instantly to the user.
func consumableSkillTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 7, Time: 2, Value: 16}},
		},
		{
			ID: 2279, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "MANAHEAL_PERCENT", Potion: true, Power: 20, HitTime: 0,
		},
	}), store)
}

// readInventoryUpdate asserts the next frame is InventoryUpdate carrying one
// modified update for objectID with the remaining stack count.
func readInventoryUpdate(t *testing.T, c *fakeGameClient, objectID int32, wantCount int32) {
	t.Helper()
	reply := c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("InventoryUpdate opcode = %#x, want %#x", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	r := wire.NewReader(reply[1:])
	if n := r.ReadUint16(); n != 1 {
		t.Fatalf("InventoryUpdate update count = %d, want 1", n)
	}
	if state := r.ReadUint16(); state != 2 {
		t.Fatalf("InventoryUpdate state = %d, want modified (2)", state)
	}
	r.ReadUint16() // type2
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("InventoryUpdate object id = %d, want %d", got, objectID)
	}
	r.ReadInt32() // template id / enchant level
	if got := r.ReadInt32(); got != wantCount {
		t.Fatalf("InventoryUpdate count = %d, want %d", got, wantCount)
	}
}

func readMagicSkillUseSelf(t *testing.T, c *fakeGameClient, objectID int32, skillID, level int32) {
	t.Helper()
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("MagicSkillUse opcode = %#x, want %#x", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	r := wire.NewReader(reply[1:])
	if caster, target, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != objectID || target != objectID || sid != skillID || lvl != level {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d level %d, want %d/%d/%d/%d", caster, target, sid, lvl, objectID, objectID, skillID, level)
	}
	if hitTime, reuse := r.ReadInt32(), r.ReadInt32(); hitTime != 0 || reuse != 0 {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want 0/0", hitTime, reuse)
	}
}

// readShortBuffStatusUpdateFrame asserts the next frame is
// ShortBuffStatusUpdate carrying skillID/level/durationSeconds.
func readShortBuffStatusUpdateFrame(t *testing.T, c *fakeGameClient, skillID, level, durationSeconds int32) {
	t.Helper()
	reply := c.read()
	if reply[0] != serverpackets.OpcodeShortBuffStatusUpdate {
		t.Fatalf("opcode = %#x, want ShortBuffStatusUpdate (%#x)", reply[0], serverpackets.OpcodeShortBuffStatusUpdate)
	}
	r := wire.NewReader(reply[1:])
	if sid, lvl, dur := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); sid != skillID || lvl != level || dur != durationSeconds {
		t.Fatalf("ShortBuffStatusUpdate = skill %d level %d duration %d, want %d/%d/%d", sid, lvl, dur, skillID, level, durationSeconds)
	}
}

// TestGameClientLinkUseHealingPotionAppliesAndConsumes verifies a healing
// potion used from the item window decrements the stack by one, announces
// the instant skill use, and installs the heal-over-time effect on the
// player. A second use within the item's reuse window is rejected with the
// reuse system message rather than silently dropped, and consumes nothing.
func TestGameClientLinkUseHealingPotionAppliesAndConsumes(t *testing.T) {
	skills := consumableSkillTable(t)
	const potionTemplate int32 = 1060
	const objectID int32 = 700
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: potionTemplate, OwnerID: objID,
			Count: 5, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed potion: %v", err)
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(chars.soleObjectID(t))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}

	c.send(encodeUseItem(objectID, false))
	readInventoryUpdate(t, c, objectID, 4)
	readMagicSkillUseSelf(t, c, live.ObjectID(), 2031, 1)
	assertSystemMessageSkillFrame(t, c.read(), serverpackets.SystemMessageUseS1, 2031, 1)
	readShortBuffStatusUpdateFrame(t, c, 2031, 1, 14)

	effects := live.EffectList().All()
	if len(effects) == 0 || effects[0].Skill.ID != 2031 {
		t.Fatalf("healing potion installed effects = %+v, want one HOT effect from skill 2031", effects)
	}

	c.send(encodeUseItem(objectID, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("reuse opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageS1PreparedForReuse {
		t.Fatalf("reuse SystemMessage id = %d, want S1PreparedForReuse (%d)", id, serverpackets.SystemMessageS1PreparedForReuse)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("reuse follow-up opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 4 {
		t.Fatalf("stack count after rejected reuse = %d, want 4 (unchanged)", got)
	}
}

// TestGameClientLinkUseHealingPotionRejectsFlyingCondition verifies the
// item-window path checks a potion's item-level use condition before
// dispatching to the consumable handler. Lesser Healing Potion is rejected
// while flying and the stack remains untouched.
func TestGameClientLinkUseHealingPotionRejectsFlyingCondition(t *testing.T) {
	skills := consumableSkillTable(t)
	const potionTemplate int32 = 1060
	const objectID int32 = 702
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: potionTemplate, OwnerID: objID,
			Count: 5, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed potion: %v", err)
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(chars.soleObjectID(t))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}
	live.SetFlying(true)

	c.send(encodeUseItem(objectID, false))
	assertSystemMessageItemFrame(t, c.read(), serverpackets.SystemMessageS1CannotBeUsed, potionTemplate)

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 5 {
		t.Fatalf("stack count after flying-condition rejection = %d, want 5 (unchanged)", got)
	}
	if effects := live.EffectList().All(); len(effects) != 0 {
		t.Fatalf("flying-condition rejection installed effects = %+v, want none", effects)
	}
}

// TestGameClientLinkUseDisabledPotionRejectsBeforeConsume verifies the
// item-window path honors per-item reuse disables before the consumable
// handler can consume from the stack.
func TestGameClientLinkUseDisabledPotionRejectsBeforeConsume(t *testing.T) {
	skills := consumableSkillTable(t)
	const potionTemplate int32 = 1060
	const objectID int32 = 703
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: potionTemplate, OwnerID: objID,
			Count: 5, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed potion: %v", err)
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(chars.soleObjectID(t))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}
	live.DisableItem(objectID, time.Minute)

	c.send(encodeUseItem(objectID, false))
	assertSystemMessageSkillFrame(t, c.read(), serverpackets.SystemMessageS1PreparedForReuse, 2031, 1)
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("disabled-item follow-up opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 5 {
		t.Fatalf("stack count after disabled-item rejection = %d, want 5 (unchanged)", got)
	}
}

// TestGameClientLinkUseManaPotionRestoresAndConsumes verifies a mana potion
// used from the item window decrements the stack by one and applies the
// mana restore to the player, acknowledged to the client via a StatusUpdate
// carrying the new MP.
func TestGameClientLinkUseManaPotionRestoresAndConsumes(t *testing.T) {
	skills := consumableSkillTable(t)
	const potionTemplate int32 = 728
	const objectID int32 = 701
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: potionTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed mana potion: %v", err)
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	playerID := chars.soleObjectID(t)
	chars.updateCharacter(t, playerID, func(ch *player.Character) {
		ch.ReduceCurrentMP(20) // leave MP below max so the restore is observable
	})

	obj, ok := state.Player(playerID)
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}
	maxMP := live.MaxMPValue()
	beforeMP := live.CurrentMP()

	c.send(encodeUseItem(objectID, false))
	readInventoryUpdate(t, c, objectID, 2)
	readMagicSkillUseSelf(t, c, live.ObjectID(), 2279, 2)
	assertSystemMessageSkillFrame(t, c.read(), serverpackets.SystemMessageUseS1, 2279, 2)
	assertStatusAttrs(t, c.read(), live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentMP, Value: beforeMP + int(maxMP*20/100)},
	})

	if got := live.CurrentMP(); got <= beforeMP {
		t.Fatalf("MP after mana potion = %d, want > %d (before)", got, beforeMP)
	}
	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("mana potion stack count = %d, want 2", got)
	}
}
