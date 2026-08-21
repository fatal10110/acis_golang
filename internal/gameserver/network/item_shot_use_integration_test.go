//go:build integration

package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// shotWeaponSeed returns a newLinkedSQLGameClient seed closure
// that creates a selectable character with a D-grade sword equipped
// (SoulshotCount 1 / SpiritshotCount 1, from testItemTemplates) plus one
// shot item.
func shotWeaponSeed(t *testing.T, shotTemplate, shotObjectID int32, shotCount int) func(*gamesql.CharacterStore, *gamesql.ItemStore) {
	t.Helper()
	return func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: 800, TemplateID: 30, OwnerID: objID,
			Location: item.LocationPaperdoll, LocationData: 7, // RHand
		}); err != nil {
			t.Fatalf("seed weapon: %v", err)
		}
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: shotObjectID, TemplateID: shotTemplate, OwnerID: objID,
			Count: shotCount, Location: item.LocationInventory,
		}); err != nil {
			t.Fatalf("seed shot: %v", err)
		}
	}
}

// TestGameClientLinkUseSoulshotChargesWeaponAndConsumes verifies a
// soulshot used directly from the item window charges the equipped
// weapon, consumes the weapon's soulshot count, announces
// ENABLED_SOULSHOT, and broadcasts the charge's visual MagicSkillUse.
func TestGameClientLinkUseSoulshotChargesWeaponAndConsumes(t *testing.T) {
	const shotObjectID int32 = 801
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, shotWeaponSeed(t, 1463, shotObjectID, 10), 1)

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

	c.Send(encodeUseItem(shotObjectID, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageEnabledSoulshot {
		t.Fatalf("SystemMessage id = %d, want EnabledSoulshot (%d)", id, serverpackets.SystemMessageEnabledSoulshot)
	}

	reply = c.Read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	r = wire.NewReader(reply[1:])
	if caster, target, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != live.ObjectID() || target != live.ObjectID() || sid != 2150 || lvl != 1 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d level %d, want %d/%d/2150/1", caster, target, sid, lvl, live.ObjectID(), live.ObjectID())
	}

	inventoryUpdatesFor(t, state).Tick()
	readInventoryUpdate(t, c, shotObjectID, 9)

	if !live.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after use")
	}
	if got := live.Inventory().ItemByObjectID(shotObjectID).Snapshot().Count; got != 9 {
		t.Fatalf("shot stack count = %d, want 9 (consumed weapon SoulshotCount=1)", got)
	}
}

// TestGameClientLinkUseSoulshotAlreadyChargedIsSilent verifies a second
// soulshot use while already charged produces no reply at all, matching
// the reference's own unconditional silence for that case.
func TestGameClientLinkUseSoulshotAlreadyChargedIsSilent(t *testing.T) {
	const shotObjectID int32 = 802
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, nil, shotWeaponSeed(t, 1463, shotObjectID, 10), 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeUseItem(shotObjectID, false))
	for c.ReadWithTimeout(time.Second) != nil {
	}

	c.Send(encodeUseItem(shotObjectID, false))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("second use while charged replied %x, want no reply at all", reply)
	}
}

// TestGameClientLinkUseSoulshotGradeMismatchAnswersActionFailed verifies a
// grade-mismatched soulshot (a C-grade shot against a D-grade weapon)
// answers the mismatch message plus ActionFailed, and consumes nothing.
func TestGameClientLinkUseSoulshotGradeMismatchAnswersActionFailed(t *testing.T) {
	const shotObjectID int32 = 803
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, nil, shotWeaponSeed(t, 1464, shotObjectID, 10), 1) // 1464: C-grade, weapon (30) is D-grade

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeUseItem(shotObjectID, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageSoulshotsGradeMismatch {
		t.Fatalf("SystemMessage id = %d, want SoulshotsGradeMismatch (%d)", id, serverpackets.SystemMessageSoulshotsGradeMismatch)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}

// TestGameClientLinkUseSoulshotNotEnoughWithAutoEnabledDisablesAutoShot
// verifies that when a direct-use soulshot fails to consume (empty stack)
// while the item is auto-enabled, the reference's disableAutoShot branch
// runs: the NOT_ENOUGH_SOULSHOTS message is suppressed, but the client
// still gets ExAutoSoulShot(itemId, 0) and AUTO_USE_OF_S1_CANCELLED
// (Player.java:5106-5117), plus ActionFailed for the pending click. It
// also verifies the weapon is left uncharged (SoulShots.java:49-62): a
// prior direct-use bug charged the weapon before destroying the stack.
func TestGameClientLinkUseSoulshotNotEnoughWithAutoEnabledDisablesAutoShot(t *testing.T) {
	const shotObjectID int32 = 804
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, shotWeaponSeed(t, 1463, shotObjectID, 0), 1)

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

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	c.Read() // ExAutoSoulShot(1463, true)
	c.Read() // SystemMessage UseOfItemWillBeAuto

	c.Send(encodeUseItem(shotObjectID, false))

	assertExAutoSoulShotFrame(t, c.Read(), 1463, false)

	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageAutoUseOfItemCancelled {
		t.Fatalf("SystemMessage id = %d, want AutoUseOfItemCancelled (%d)", id, serverpackets.SystemMessageAutoUseOfItemCancelled)
	}

	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}

	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("unexpected extra reply %x, want none (NOT_ENOUGH_SOULSHOTS suppressed while auto-enabled)", reply)
	}

	if live.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = true after a failed destroy, want the weapon left uncharged")
	}
	if live.AutoSoulShotEnabled(1463) {
		t.Fatal("AutoSoulShotEnabled(1463) = true after disableAutoShot, want false")
	}
}
