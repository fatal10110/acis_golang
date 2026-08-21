//go:build integration

package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/testsupport"
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
func readMagicSkillUseSelfWithReuse(t *testing.T, c *testsupport.ScriptedClient, objectID int32, skillID, level, wantReuse int32) {
	t.Helper()
	reply := c.Read()
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
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}

	c.Send(encodeUseItem(objectID, false))
	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("next opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}

	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, objectID, 2)

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("scroll stack count after cast = %d, want 2", got)
	}
}

func TestGameClientLinkUseScrollDefersUntilAttackFinishes(t *testing.T) {
	skills := itemAICastSkillTable(t)
	const scrollTemplate int32 = 736
	const objectID int32 = 708
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}
	if err := live.attack.DoAttack(newTestHostileNPC(t, 709)); err != nil {
		t.Fatalf("start attack: %v", err)
	}
	c.Send(encodeUseItem(objectID, false))
	for {
		reply := c.Read()
		if reply[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("item cast started before the active attack finished")
		}
		if reply[0] == serverpackets.OpcodeActionFailed {
			break
		}
	}
	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 3 {
		t.Fatalf("scroll stack during deferred cast = %d, want 3", got)
	}

	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)
	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("scroll stack after deferred cast starts = %d, want 2", got)
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
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	sqlSoleObjectID(t, chars)

	c.Send(encodeUseItem(objectID, false))
	reply := c.Read()
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
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
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
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 1, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}

	c.Send(encodeUseItem(objectID, false))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("opcode = %#x, want SetupGauge (%#x)", reply[0], serverpackets.OpcodeSetupGauge)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}

	// Drain every remaining MP before the Hit phase fires 400ms after
	// launch, so Controller.hitLocked rejects even the 1 MP final cost
	// with ErrNotEnoughMP.
	live.Character.ReduceCurrentMP(live.Character.CurrentMP())

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("hit-failure opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageNotEnoughMP {
		t.Fatalf("SystemMessage id = %d, want not enough MP", id)
	}

	assertStatusAttrs(t, c.Read(), live.ObjectID(), []serverpackets.StatusAttribute{
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

func TestGameClientLinkUseScrollRevalidatesTargetAtLaunch(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{{
		ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "PDAM", StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 5000,
		EffectRange: 1,
	}}), store)
	const scrollTemplate int32 = 736
	const objectID int32 = 706
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 1, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}
	target := newTestHostileNPC(t, 707)
	state.Spawn(target, 100, 0, 0, 0)
	live.SetTargetTracked(target)
	c.Read() // NPCInfo

	c.Send(encodeUseItem(objectID, false))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("opcode = %#x, want SetupGauge (%#x)", reply[0], serverpackets.OpcodeSetupGauge)
	}
	assertStaticSystemMessageFrame(t, c.Read(), serverpackets.SystemMessageDistTooFarCastingStopped)
}

// TestGameClientLinkUseScrollRejectsReuse verifies a still-cooling
// item-carried skill answers the same reuse rejection a player skill cast
// produces, and does not consume the item.
func TestGameClientLinkUseScrollRejectsReuse(t *testing.T) {
	skills := itemAICastSkillTable(t)
	const scrollTemplate int32 = 736
	const objectID int32 = 703
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: scrollTemplate, OwnerID: objID,
			Count: 3, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed scroll: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}

	c.Send(encodeUseItem(objectID, false))
	readMagicSkillUseSelfWithReuse(t, c, live.ObjectID(), 2013, 1, 5000)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}
	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, objectID, 2)

	c.Send(encodeUseItem(objectID, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("reuse opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageS1PreparedForReuse {
		t.Fatalf("reuse SystemMessage id = %d, want S1PreparedForReuse (%d)", id, serverpackets.SystemMessageS1PreparedForReuse)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("reuse follow-up opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 2 {
		t.Fatalf("stack count after rejected reuse = %d, want 2 (unchanged)", got)
	}
}
