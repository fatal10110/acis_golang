package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootShotRig boots a character with the D-grade sword equipped plus a shot
// stack, returning the server and the shot object id.
func bootShotRig(t *testing.T, shotTemplate int32, shotCount int32) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	shot := srv.GiveItem(t, objID, shotTemplate, shotCount)
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)
	return srv, shot
}

// TestUseSoulshotChargesWeaponAndConsumes drives a soulshot used directly
// from the item window: EnabledSoulshot announcement, the charge's visual
// MagicSkillUse, and one unit consumed per the weapon's soulshot count in
// both the packet stream and the items row. A second use while already
// charged is silent.
func TestUseSoulshotChargesWeaponAndConsumes(t *testing.T) {
	srv, shot := bootShotRig(t, 1463, 10)
	c := srv.Client
	objID := srv.SoleObjectID(t)

	c.Send(encodeUseItem(shot, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "EnabledSoulshot")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessageEnabledSoulshot {
		t.Fatalf("message id = %d, want EnabledSoulshot (%d)", id, serverpackets.SystemMessageEnabledSoulshot)
	}
	assertMagicSkillUseSelf(t, c.Read(), objID, 2150, 1, 0, 0)
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, shot, 9)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, shot); inst.Count != 9 {
		t.Fatalf("persisted shot count = %d, want 9", inst.Count)
	}

	drainUntilQuiet(t, c)
	c.Send(encodeUseItem(shot, false))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("second use while charged replied %x, want no reply at all", reply)
	}
}

// TestUseSoulshotGradeMismatchIsRejected pins the grade gate: a C-grade
// soulshot against a D-grade weapon answers the mismatch message plus
// ActionFailed and consumes nothing.
func TestUseSoulshotGradeMismatchIsRejected(t *testing.T) {
	srv, shot := bootShotRig(t, 1464, 10)
	c := srv.Client

	c.Send(encodeUseItem(shot, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSoulshotsGradeMismatch)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "grade mismatch")
	barrier(t, c)
}

// TestAutoSoulShotToggle pins RequestAutoSoulShot: enabling and disabling a
// known inventory shot echoes ExAutoSoulShot plus its system message each
// way.
func TestAutoSoulShotToggle(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 1463, 100)
	startInWorld(t, c)

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 1463)

	c.Send(encodeRequestAutoSoulShot(1463, 0))
	assertExAutoSoulShot(t, c.Read(), 1463, false)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageAutoUseOfItemCancelled, 1463)
}

// TestUseSoulshotNotEnoughWithAutoDisablesAuto pins the failed-consume path:
// a direct-use soulshot with an empty stack while auto-enabled suppresses
// the not-enough message but still disables auto use (ExAutoSoulShot off,
// cancellation message, ActionFailed for the click).
func TestUseSoulshotNotEnoughWithAutoDisablesAuto(t *testing.T) {
	srv, shot := bootShotRig(t, 1463, 0)
	c := srv.Client

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 1463)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(shot, false))
	assertExAutoSoulShot(t, c.Read(), 1463, false)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageAutoUseOfItemCancelled, 1463)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "failed consume")
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("unexpected extra reply %x, want none (not-enough suppressed while auto-enabled)", reply)
	}
}

// TestUseBeastSoulshotChargesSummonAndConsumes drives a beast soulshot used
// with an active servitor: PetUsesS1 announcement, the charge visual cast by
// the servitor rather than the player, and the servitor's per-hit count
// consumed from the stack.
func TestUseBeastSoulshotChargesSummonAndConsumes(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	shot := srv.GiveItem(t, objID, 6645, 10)
	startInWorld(t, c)

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID: srv.NewObjectID(),
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.State.AddSummon(objID, servitor)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(shot, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "PetUsesS1")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessagePetUsesS1 {
		t.Fatalf("message id = %d, want PetUsesS1 (%d)", id, serverpackets.SystemMessagePetUsesS1)
	}

	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeMagicSkillUse, "beast charge MagicSkillUse")
	r := wire.NewReader(reply[1:])
	caster, target, sid, lvl := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != servitor.ObjectID() || target != servitor.ObjectID() || sid != 2033 || lvl != 1 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d level %d, want %d/%d/2033/1",
			caster, target, sid, lvl, servitor.ObjectID(), servitor.ObjectID())
	}

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, shot, 5)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, shot); inst.Count != 5 {
		t.Fatalf("persisted beast shot count = %d, want 5", inst.Count)
	}
}

// TestUseBeastSoulshotWithoutSummonIsRejected pins the no-summon rejection:
// PetsNotAvailableAtThisTime only, matching the reference handler, and
// nothing consumed.
func TestUseBeastSoulshotWithoutSummonIsRejected(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	shot := srv.GiveItem(t, objID, 6645, 10)
	startInWorld(t, c)

	c.Send(encodeUseItem(shot, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessagePetsNotAvailableAtThisTime)
	if inst := mustFindItem(t, srv, objID, shot); inst.Count != 10 {
		t.Fatalf("beast shot count after rejection = %d, want 10", inst.Count)
	}
}

// TestAutoSoulShotIgnoresUnknownAndFishingShots pins RequestAutoSoulShot's
// ignore branches: an unknown item id and a fishing-shot item both produce
// no reply at all and never enable auto use.
func TestAutoSoulShotIgnoresUnknownAndFishingShots(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 6535, 100) // fishing shot
	startInWorld(t, c)

	for _, itemID := range []int32{999999, 6535} {
		c.Send(encodeRequestAutoSoulShot(itemID, 1))
		if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
			t.Fatalf("auto-soulshot for %d replied %x, want silence", itemID, reply)
		}
	}
	drainUntilQuiet(t, c)
}
