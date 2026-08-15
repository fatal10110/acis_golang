//go:build integration

package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// beastShotSeed returns a newLinkedSQLGameClient seed closure
// that creates a selectable character plus one beast shot item; the active
// summon itself is added to world state after enter-world since it has no
// item-store representation.
func beastShotSeed(t *testing.T, shotTemplate, shotObjectID int32, shotCount int) func(*gamesql.CharacterStore, *gamesql.ItemStore) {
	t.Helper()
	return func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: shotObjectID, TemplateID: shotTemplate, OwnerID: objID,
			Count: shotCount, Location: item.LocationInventory,
		}); err != nil {
			t.Fatalf("seed beast shot: %v", err)
		}
	}
}

// TestGameClientLinkUseBeastSoulshotChargesSummonAndConsumes verifies a
// beast soulshot used directly from the item window charges the caster's
// active servitor, consumes the servitor's SSCount per-hit count, announces
// PET_USES_S1, and broadcasts the charge's visual MagicSkillUse cast by the
// servitor rather than the player.
func TestGameClientLinkUseBeastSoulshotChargesSummonAndConsumes(t *testing.T) {
	const shotObjectID int32 = 900
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, beastShotSeed(t, 6645, shotObjectID, 10), 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	objID := sqlSoleObjectID(t, chars)
	servitor := summon.NewServitor(summon.ServitorConfig{
		ObjectID: 500,
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5},
	})
	state.AddSummon(objID, servitor)

	c.send(encodeUseItem(shotObjectID, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessagePetUsesS1 {
		t.Fatalf("SystemMessage id = %d, want PetUsesS1 (%d)", id, serverpackets.SystemMessagePetUsesS1)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	r = wire.NewReader(reply[1:])
	if caster, target, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != servitor.ObjectID() || target != servitor.ObjectID() || sid != 2033 || lvl != 1 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d level %d, want %d/%d/2033/1", caster, target, sid, lvl, servitor.ObjectID(), servitor.ObjectID())
	}

	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, shotObjectID, 5)

	if !servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after use")
	}
}

// TestGameClientLinkUseBeastSoulshotNoSummonAnswersRejection verifies a
// beast soulshot used without an active summon answers
// PETS_ARE_NOT_AVAILABLE_AT_THIS_TIME only (no ActionFailed, matching
// BeastSoulShots.java) and consumes nothing.
func TestGameClientLinkUseBeastSoulshotNoSummonAnswersRejection(t *testing.T) {
	const shotObjectID int32 = 901
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, nil, beastShotSeed(t, 6645, shotObjectID, 10), 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeUseItem(shotObjectID, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessagePetsNotAvailableAtThisTime {
		t.Fatalf("SystemMessage id = %d, want PetsNotAvailableAtThisTime (%d)", id, serverpackets.SystemMessagePetsNotAvailableAtThisTime)
	}
}
