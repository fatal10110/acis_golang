package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// itemAICastSkillTable seeds a self-target, non-potion carried skill (the
// Scroll: Escape template in testItemTemplates) so the item-window AI-cast
// path (as opposed to the instant-cast potion path) has something to
// resolve and cast.
func itemAICastSkillTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "TELEPORT", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 5000,
		},
	}), store)
}

// readMagicSkillUseSelfWithReuse asserts the next frame is MagicSkillUse
// cast by and on objectID for skillID/level, carrying wantReuse as the
// installed reuse delay (unlike the instant-cast potion path, an
// AI-dispatched item skill reports its real reuse delay here).
func readMagicSkillUseSelfWithReuse(t *testing.T, c *fakeGameClient, objectID int32, skillID, level, wantReuse int32) {
	t.Helper()
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("MagicSkillUse opcode = %#x, want %#x", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	r := wire.NewReader(reply[1:])
	if caster, target, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != objectID || target != objectID || sid != skillID || lvl != level {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d level %d, want %d/%d/%d/%d", caster, target, sid, lvl, objectID, objectID, skillID, level)
	}
	if hitTime, reuse := r.ReadInt32(), r.ReadInt32(); hitTime != 0 || reuse != wantReuse {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want 0/%d", hitTime, reuse, wantReuse)
	}
}

// TestGameClientLinkUseScrollRunsAICastAndConsumes verifies a non-potion
// item-carried skill (a scroll) used from the item window runs through the
// AI cast path rather than the instant-cast path: it announces the skill
// use against the player's own object (self target), then consumes one
// unit of the item once the cast starts.
func TestGameClientLinkUseScrollRunsAICastAndConsumes(t *testing.T) {
	skills := itemAICastSkillTable(t)
	const scrollTemplate int32 = 736
	const objectID int32 = 702
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
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
	readInventoryUpdate(t, c, objectID, 2)
	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)
	assertSystemMessageSkillFrame(t, c.read(), serverpackets.SystemMessageUseS1, 2013, 1)

	reply := c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("next opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("scroll stack count after cast = %d, want 2", got)
	}
}

// TestGameClientLinkUseScrollRejectsReuse verifies a still-cooling
// item-carried skill answers the same reuse rejection a player skill cast
// produces, and does not consume the item.
func TestGameClientLinkUseScrollRejectsReuse(t *testing.T) {
	skills := itemAICastSkillTable(t)
	const scrollTemplate int32 = 736
	const objectID int32 = 703
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
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
	readInventoryUpdate(t, c, objectID, 2)
	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)
	assertSystemMessageSkillFrame(t, c.read(), serverpackets.SystemMessageUseS1, 2013, 1)
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
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

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("stack count after rejected reuse = %d, want 2 (unchanged)", got)
	}
}
