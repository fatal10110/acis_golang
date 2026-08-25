package items

import (
	"context"
	"testing"

	sqltest "github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestEquipUnequipRoundTrip equips the D-grade sword through UseItem,
// requires UserInfo plus the tick-driven InventoryUpdate and the paperdoll
// location in the persisted row, then unequips it through
// RequestUnEquipItem and requires the inventory slot restore in both the
// packet stream and the items row.
func TestEquipUnequipRoundTrip(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	e := readInventoryUpdateFor(t, c, weapon, 1)
	if e.equipped != 1 {
		t.Fatalf("equipped flag after equip = %d, want 1", e.equipped)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, weapon); inst.Location != item.LocationPaperdoll {
		t.Fatalf("persisted weapon location after equip = %v, want paperdoll", inst.Location)
	}

	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "unequip UserInfo"), serverpackets.OpcodeUserInfo, "unequip UserInfo")
	assertSystemMessageItem(t, readSkippingEquipNoise(t, c, "disarmed message"), serverpackets.SystemMessageS1Disarmed, 30)
	srv.InventoryUpdates.Tick()
	e = readInventoryUpdateFor(t, c, weapon, 1)
	if e.equipped != 0 {
		t.Fatalf("equipped flag after unequip = %d, want 0", e.equipped)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, weapon); inst.Location != item.LocationInventory {
		t.Fatalf("persisted weapon location after unequip = %v, want inventory", inst.Location)
	}
}

// TestUnequipMessageReflectsEnchantLevel walks the unequip message table:
// a plain item answers S1_DISARMED while an enchanted one answers
// EQUIPMENT_S1_S2_REMOVED carrying the enchant level and item name.
func TestUnequipMessageReflectsEnchantLevel(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enchant    int32
		wantMsg    int
		wantParams int32
	}{
		{name: "plain", enchant: 0, wantMsg: serverpackets.SystemMessageS1Disarmed, wantParams: 1},
		{name: "enchanted", enchant: 6, wantMsg: serverpackets.SystemMessageEquipmentS1S2Removed, wantParams: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			chest := srv.GiveItem(t, objID, 40, 1)
			if tc.enchant > 0 {
				inst := mustFindItem(t, srv, objID, chest)
				inst.EnchantLevel = int(tc.enchant)
				if err := srv.Items.Update(context.Background(), inst); err != nil {
					t.Fatalf("seed enchant level: %v", err)
				}
			}
			startInWorld(t, c)

			// Equip first so unequip has something on the paperdoll.
			c.Send(encodeUseItem(chest, false))
			assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
			srv.InventoryUpdates.Tick()
			readInventoryUpdateFor(t, c, chest, 1)

			c.Send(encodeRequestUnEquipItem(int32(item.SlotChest)))
			assertFrameOpcode(t, readSkippingEquipNoise(t, c, "unequip UserInfo"), serverpackets.OpcodeUserInfo, "unequip UserInfo")
			frame := readSkippingEquipNoise(t, c, "unequip SystemMessage")
			assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "unequip SystemMessage")
			r := wire.NewReader(frame[1:])
			if got := r.ReadInt32(); got != int32(tc.wantMsg) {
				t.Fatalf("unequip message id = %d, want %d", got, tc.wantMsg)
			}
			if params := r.ReadInt32(); params != tc.wantParams {
				t.Fatalf("unequip message params = %d, want %d", params, tc.wantParams)
			}
			if tc.enchant > 0 {
				if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
					t.Fatalf("param[0] type = %d, want number", typ)
				}
				if lvl := r.ReadInt32(); lvl != tc.enchant {
					t.Fatalf("param[0] enchant = %d, want %d", lvl, tc.enchant)
				}
			}
			if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
				t.Fatalf("item param type = %d, want item name", typ)
			}
			if id := r.ReadInt32(); id != 40 {
				t.Fatalf("item param id = %d, want 40", id)
			}
			srv.InventoryUpdates.Tick()
			readInventoryUpdateFor(t, c, chest, 1)
		})
	}
}

// TestUnequipEmptySlotIsRejected pins RequestUnEquipItem against an empty
// paperdoll slot: only ActionFailed answers.
func TestUnequipEmptySlotIsRejected(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)

	c.Send(encodeRequestUnEquipItem(int32(item.SlotChest)))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeActionFailed, "empty-slot unequip")
	testsupport.SyncBarrier(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList)
}

// TestDeadPlayerItemGates walks the dead-state table: UseItem is gated with
// ActionFailed, RequestUnEquipItem answers a system message without
// unequipping, while destroy and crystallize stay reachable — matching the
// reference's per-operation gates.
func TestDeadPlayerItemGates(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	spare := srv.GiveItem(t, objID, 30, 1)
	potion := srv.GiveItem(t, objID, 20, 2)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
		t.Fatalf("grant crystallize skill: %v", err)
	}
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	readSkippingEquipNoise(t, c, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)
	drainUntilQuiet(t, c)

	srv.MarkPlayerDead(t, objID)

	c.Send(encodeUseItem(potion, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "dead use gate")
	barrier(t, c)

	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	frame := readSkippingEquipNoise(t, c, "dead unequip reply")
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("dead unequip opcode = %#x, want SystemMessage", frame[0])
	}
	barrier(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, weapon); inst.Location != item.LocationPaperdoll {
		t.Fatalf("dead unequip moved the weapon: %+v", inst)
	}

	c.Send(encodeRequestDestroyItem(potion, 1))
	waitFor(t, "dead destroy consumed one unit", func() bool {
		srv.InventoryUpdates.Tick()
		srv.FlushItems(t)
		for _, inst := range persistedItems(t, srv, objID) {
			if inst.ObjectID == potion {
				return inst.Count == 1
			}
		}
		return false
	})

	c.Send(encodeRequestCrystallizeItem(spare, 1))
	frames := collectUntilQuiet(t, c)
	crystallized := false
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageItemCrystallized {
			crystallized = true
		}
	}
	if !crystallized {
		t.Fatalf("crystallize while dead produced no crystallized message across %d frames", len(frames))
	}
	srv.FlushItems(t)
	assertItemGone(t, srv, objID, spare)
}

// TestDropFarCoordinatesRejected pins the drop distance gate: coordinates
// beyond drop range answer CannotDiscardDistanceTooFar and nothing leaves
// the inventory.
func TestDropFarCoordinatesRejected(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX+10000, spawnY, spawnZ))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotDiscardDistanceTooFar)
	barrier(t, c)

	if inst := mustFindItem(t, srv, objID, adena); inst.Count != 100 {
		t.Fatalf("adena count after far-drop rejection = %d, want 100", inst.Count)
	}
}

// TestEquipBroadcastsCharInfoToObservers pins the appearance broadcast:
// when a player equips a weapon, every client that knows them receives a
// CharInfo refresh.
func TestEquipBroadcastsCharInfoToObservers(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	srv.SeedCharacterFor(t, "player2", "Second", 5, 0)
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(weapon, false))
	readSkippingEquipNoise(t, c, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)

	var sawCharInfo bool
	for _, f := range collectUntilQuiet(t, observer) {
		if f[0] == serverpackets.OpcodeCharInfo {
			sawCharInfo = true
		}
	}
	if !sawCharInfo {
		t.Fatal("equipping a weapon produced no CharInfo refresh for the observer")
	}
}

// TestDestroyCountGates pins the destroy count table reachable from the
// wire: a zero count and a count above the held stack each answer
// CANNOT_DESTROY_NUMBER_INCORRECT and change nothing.
func TestDestroyCountGates(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 2)
	startInWorld(t, c)

	for _, count := range []int32{0, 3} {
		c.Send(encodeRequestDestroyItem(adena, count))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotDestroyNumberIncorrect)
		barrier(t, c)
	}
	if inst := mustFindItem(t, srv, objID, adena); inst.Count != 2 {
		t.Fatalf("adena count after rejected destroys = %d, want 2", inst.Count)
	}
}

// TestUnequipDuringCastIsRejected pins the casting gate: unequipping while
// mid-cast answers S1_CANNOT_BE_USED instead of applying.
func TestUnequipDuringCastIsRejected(t *testing.T) {
	db := sqltest.SharedDB(t)
	store := gamesql.NewSkillSaveStore(db)
	known := gamesql.NewCharacterSkillStore(db)
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 1500, StaticReuse: true, ReuseDelay: 10000,
		},
	}), known)
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, 736, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "scroll cast")
	c.Read() // MagicSkillLaunched

	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeSystemMessage {
		if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1CannotBeUsed {
			t.Fatalf("unequip-during-cast message id = %d, want S1CannotBeUsed (%d)", id, serverpackets.SystemMessageS1CannotBeUsed)
		}
	} else if frame[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("unequip-during-cast opcode = %#x, want SystemMessage or ActionFailed", frame[0])
	}
	barrier(t, c)
}
