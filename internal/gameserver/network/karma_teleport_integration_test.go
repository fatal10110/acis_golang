//go:build integration

package network

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// seedSelectableKarmaCharacter is seedSelectableCharacter with karma set
// before the character is stored, so the karma-teleport guard tests set
// karma during single-threaded seeding instead of racing the connection's
// own goroutine by mutating live.Character after EnterWorld.
func seedSelectableKarmaCharacter(t *testing.T, chars *gamesql.CharacterStore, account, name string, level, karma int) int32 {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, account, name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.KarmaPoints = karma
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	// Create's INSERT omits karma (a character never starts play with karma
	// set; it's earned in-game and written by Save) — Save it explicitly so
	// this test's pre-set karma survives the store round-trip.
	if err := chars.Save(context.Background(), ch); err != nil {
		t.Fatalf("seed character karma: %v", err)
	}
	return ch.ID
}

// recallSkillTable seeds one RECALL-type skill (Return to Player, item
// 1060's attached skill in testItemTemplates) so both the item-use and
// direct-cast karma-teleport guards have a real skill to resolve.
func recallSkillTable(t *testing.T) (*skillstate.Persistence, *memorySkillSaveStore) {
	t.Helper()
	store := newMemorySkillSaveStore()
	pers := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "RECALL", Potion: true, HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
		},
	}), store)
	return pers, store
}

// TestGameClientLinkUseItemBlockedByKarmaTeleport verifies a player with
// karma cannot use an item whose attached skill is RECALL/TELEPORT when
// KarmaPlayerCanTeleport is false: the use is silently rejected (no packet,
// mirroring the reference's bare `return`) and the item stack is untouched.
func TestGameClientLinkUseItemBlockedByKarmaTeleport(t *testing.T) {
	skills, _ := recallSkillTable(t)
	const potionTemplate int32 = 1060
	const objectID int32 = 700
	var playerObjID int32
	c, _, _, state := newLinkedSQLGameClientWithKarmaPlayerCanTeleport(t, false, skills, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		playerObjID = seedSelectableKarmaCharacter(t, chars, "player1", "Newbie", 5, 1)
		if err := items.Create(context.Background(), playerObjID, item.Instance{
			ObjectID: objectID, TemplateID: potionTemplate, OwnerID: playerObjID,
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

	obj, ok := state.Player(playerObjID)
	if !ok {
		t.Fatal("player not in world state after enter")
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatal("world player is not a *livePlayer")
	}

	c.send(encodeUseItem(objectID, false))
	if reply := c.readWithTimeout(500 * time.Millisecond); reply != nil {
		t.Fatalf("karma-blocked item use reply = opcode %#x, want no reply (silent reject)", reply[0])
	}

	if got := live.Inventory().ItemByObjectID(objectID).Snapshot().Count; got != 5 {
		t.Fatalf("stack count after blocked karma-teleport use = %d, want 5 (unchanged)", got)
	}
}

// TestGameClientLinkMagicSkillUseRecallBlockedByKarma verifies a player with
// karma cannot directly cast a RECALL skill when KarmaPlayerCanTeleport is
// false: the client gets ActionFailed and no cast starts.
func TestGameClientLinkMagicSkillUseRecallBlockedByKarma(t *testing.T) {
	skills, store := recallSkillTable(t)
	c, _, _, _ := newLinkedSQLGameClientWithKarmaPlayerCanTeleport(t, false, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID := seedSelectableKarmaCharacter(t, chars, "player1", "Newbie", 5, 1)
		store.seedKnown(objID, 0, player.SkillLevels{2031: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(2031, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("blocked recall cast opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	if reply := c.readWithTimeout(200 * time.Millisecond); reply != nil {
		t.Fatalf("unexpected extra reply after blocked recall cast = opcode %#x", reply[0])
	}
}
