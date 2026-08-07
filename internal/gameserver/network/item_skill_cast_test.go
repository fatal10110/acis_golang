package network

import (
	"context"
	"testing"
	"time"

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
	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)

	reply := c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("next opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}

	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, objectID, 2)

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("scroll stack count after cast = %d, want 2", got)
	}
}

// TestGameClientLinkUseScrollWithSharedGroupSendsExUseSharedGroupItem
// verifies an item-carried skill from an item with a shared-reuse group
// drives the client's shared-reuse HUD packet, carrying the longer of the
// skill's own reuse delay and the item's.
func TestGameClientLinkUseScrollWithSharedGroupSendsExUseSharedGroupItem(t *testing.T) {
	skills := itemAICastSkillTable(t)
	const scrollTemplate int32 = 737 // shared group 5, item reuse 9000ms > skill's 5000ms
	const objectID int32 = 704
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
	chars.soleObjectID(t)

	c.send(encodeUseItem(objectID, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	r := wire.NewReader(reply[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExUseSharedGroupItem {
		t.Fatalf("extended sub-opcode = %#x, want ExUseSharedGroupItem (%#x)", sub, serverpackets.OpcodeExUseSharedGroupItem)
	}
	if itemID, group, remain, total := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); itemID != scrollTemplate || group != 5 || remain != 9 || total != 9 {
		t.Fatalf("ExUseSharedGroupItem = item %d group %d remain %d total %d, want %d/5/9/9", itemID, group, remain, total, scrollTemplate)
	}

	// Drain the AI cast's own frames (MagicSkillUse, MagicSkillLaunched)
	// before the tick-driven InventoryUpdate that now follows them, whatever
	// their exact content — this test only pins the shared-reuse packet and
	// the eventual stack count.
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}

	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, objectID, 2)
}

// TestGameClientLinkUseScrollHitPhaseCostFailureSendsReasonAndStatusUpdate
// covers the item-cast path's Failed hook (network/item_skill_cast.go:111-114)
// at the network level: the network suite otherwise only exercises the
// player-initiated path's Failed hook (magic_skill_test.go), leaving the
// item-carried wiring of the same hook untested, per the PR #1005 review's
// finding that the removed unit test's coverage claim was overstated. A scroll
// with a hitTime long enough to schedule an async Hit phase (500ms, matching
// CreatureCast.onMagicHitTimer:242-294's split from the launch phase) starts
// with enough MP to pass the pre-cast gate, then has its MP drained below the
// final cost before Hit fires — mirroring
// model/actor/cast/schedule_test.go:TestScheduleFailedHitStopsBeforeFinish at
// the network level — so the Hit-phase MP check (Controller.hitLocked) fails
// and the item path must still answer with the reason message and a
// StatusUpdate reflecting the drained MP.
func TestGameClientLinkUseScrollHitPhaseCostFailureSendsReasonAndStatusUpdate(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 5000,
			MPConsume: 1,
		},
	}), store)
	const scrollTemplate int32 = 736
	const objectID int32 = 705
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 1, Location: item.LocationInventory, ManaLeft: -1,
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
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("opcode = %#x, want SetupGauge (%#x)", reply[0], serverpackets.OpcodeSetupGauge)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}

	// Drain every remaining MP before the Hit phase fires 400ms after
	// launch, so Controller.hitLocked rejects even the 1 MP final cost
	// with ErrNotEnoughMP.
	live.Character.ReduceCurrentMP(live.Character.CurrentMP())

	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("hit-failure opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageNotEnoughMP {
		t.Fatalf("SystemMessage id = %d, want not enough MP", id)
	}

	assertStatusAttrs(t, c.read(), live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentMP, Value: 0},
	})

	deadline := time.Now().Add(2 * time.Second)
	for live.cast.CastingNow() {
		if time.Now().After(deadline) {
			t.Fatal("cast controller still casting after Hit-phase failure, want stopped")
		}
		time.Sleep(5 * time.Millisecond)
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
	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}
	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, objectID, 2)

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
