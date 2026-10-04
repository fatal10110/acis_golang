package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// addAutoShotServitor puts a servitor spending ss beast soulshots and sps
// beast spiritshots per charge out for objID.
func addAutoShotServitor(t *testing.T, srv *gameservertest.Server, objID int32, ss, sps int) *summon.Actor {
	t.Helper()
	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID: srv.NewObjectID(),
		Level:    44,
		Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200, SSCount: ss, SPSCount: sps},
	})
	if err != nil {
		t.Fatal(err)
	}
	servitor.SetQueue(srv.PlayerQueue(t, objID))
	srv.State.AddSummon(objID, servitor)
	drainUntilQuiet(t, srv.Client)
	return servitor
}

// assertNoReply fails when anything reaches the client within 300ms.
func assertNoReply(t *testing.T, srv *gameservertest.Server, what string) {
	t.Helper()
	if reply := srv.Client.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("%s: unexpected extra reply %x", what, reply)
	}
}

// TestAutoSoulShotEnableChargesMatchingWeapon pins
// RequestAutoSoulShot.java:87-103 for a grade match: ExAutoSoulShot on, then
// the weapon charged at once from the auto-use shots (ENABLED_SOULSHOT and
// the charge MagicSkillUse, one shot spent), then USE_OF_S1_WILL_BE_AUTO.
func TestAutoSoulShotEnableChargesMatchingWeapon(t *testing.T) {
	t.Parallel()
	srv, _ := bootShotRig(t, 1463, 10)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageEnabledSoulshot)
	assertMagicSkillUseSelf(t, c.Read(), objID, 2150, 1, 0, 0)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 1463)
	drainUntilQuiet(t, c)
	if got := carriedCount(t, srv, objID, 1463); got != 9 {
		t.Fatalf("soulshots after the activation charge = %d, want 9", got)
	}
	if !autoSoulShotEnabled(t, srv, objID, 1463) {
		t.Fatal("auto use is off after a matching enable")
	}
}

// TestAutoSpiritshotEnableGradeMismatchStillActivates pins
// RequestAutoSoulShot.java:92-103 for a mismatch: auto use still turns on,
// SPIRITSHOTS_GRADE_MISMATCH replaces the charge, and nothing is spent.
// Holding no weapon is a mismatch.
func TestAutoSpiritshotEnableGradeMismatchStillActivates(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 2509, 10)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestAutoSoulShot(2509, 1))
	assertExAutoSoulShot(t, c.Read(), 2509, true)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSpiritshotsGradeMismatch)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 2509)
	assertNoReply(t, srv, "grade mismatch")
	if got := carriedCount(t, srv, objID, 2509); got != 10 {
		t.Fatalf("spiritshots after a mismatched enable = %d, want 10", got)
	}
	if !autoSoulShotEnabled(t, srv, objID, 2509) {
		t.Fatal("grade mismatch left auto use off, want it on")
	}
}

// TestAutoBeastShotEnableRejectsStackBelowOneCharge pins
// RequestAutoSoulShot.java:51-66: a servitor shot stack smaller than what the
// summon spends per charge answers NOT_ENOUGH_SOULSHOTS_FOR_PET or
// NOT_ENOUGH_SPIRITSHOTS_FOR_PET alone and leaves auto use off.
func TestAutoBeastShotEnableRejectsStackBelowOneCharge(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 6645, 4)
	srv.GiveItem(t, objID, 6646, 2)
	startInWorld(t, c)
	addAutoShotServitor(t, srv, objID, 5, 3)

	for _, tc := range []struct {
		itemID int32
		msg    int
	}{
		{6645, serverpackets.SystemMessageNotEnoughSoulshotsForPet},
		{6646, serverpackets.SystemMessageNotEnoughSpiritshotsForPet},
	} {
		c.Send(encodeRequestAutoSoulShot(tc.itemID, 1))
		assertStaticSystemMessage(t, c.Read(), tc.msg)
		assertNoReply(t, srv, "short servitor shot stack")
		if autoSoulShotEnabled(t, srv, objID, tc.itemID) {
			t.Fatalf("auto use of %d turned on with a stack below one charge", tc.itemID)
		}
	}
}

// TestAutoBeastShotEnableChargesSummon pins RequestAutoSoulShot.java:67-73:
// ExAutoSoulShot on, USE_OF_S1_WILL_BE_AUTO, then the summon charged at once
// from the auto-use servitor shots (PET_USES_S1 and the charge MagicSkillUse
// cast by the servitor), spending its per-charge count.
func TestAutoBeastShotEnableChargesSummon(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 6645, 10)
	startInWorld(t, c)
	servitor := addAutoShotServitor(t, srv, objID, 5, 3)

	c.Send(encodeRequestAutoSoulShot(6645, 1))
	assertExAutoSoulShot(t, c.Read(), 6645, true)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 6645)
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "PetUsesS1")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessagePetUsesS1 {
		t.Fatalf("message id = %d, want PetUsesS1 (%d)", id, serverpackets.SystemMessagePetUsesS1)
	}
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "beast charge MagicSkillUse")
	r := wire.NewReader(frame[1:])
	if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != servitor.ObjectID() || target != caster || skill != 2033 {
		t.Fatalf("charge MagicSkillUse = %d/%d skill %d, want servitor-cast self 2033", caster, target, skill)
	}
	drainUntilQuiet(t, c)
	if got := carriedCount(t, srv, objID, 6645); got != 5 {
		t.Fatalf("beast soulshots after the activation charge = %d, want 5", got)
	}
}

// TestAutoSoulShotTogglesWhileFakeDead pins the dead gate of
// RequestAutoSoulShot.java:29: only a real death blocks the request, so a
// player playing dead still toggles auto use.
func TestAutoSoulShotTogglesWhileFakeDead(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 1463, 10)
	startInWorld(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.StartFakeDeath() })
	drainUntilQuiet(t, c)

	c.Send(encodeRequestAutoSoulShot(1463, 1))
	assertExAutoSoulShot(t, c.Read(), 1463, true)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSoulshotsGradeMismatch)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 1463)
	if !autoSoulShotEnabled(t, srv, objID, 1463) {
		t.Fatal("fake death blocked turning auto use on")
	}
}

// TestAutoBeastShotEnableChargesOwnerWeaponFirst pins
// RequestAutoSoulShot.java:72-73 with a weapon shot already on auto: turning
// on a servitor shot charges the owner's weapon (ENABLED_SOULSHOT and the
// player's charge MagicSkillUse) before the summon (PET_USES_S1 and the
// servitor's 2033), spending one weapon shot and one servitor charge.
func TestAutoBeastShotEnableChargesOwnerWeaponFirst(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	srv.GiveItem(t, objID, 1463, 10)
	srv.GiveItem(t, objID, 6645, 10)
	startInWorld(t, c)
	c.Send(encodeUseItem(weapon, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)
	// Weapon auto-shot on but the weapon still uncharged, so the servitor
	// enable is what charges it.
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetAutoSoulShot(1463, true) })
	servitor := addAutoShotServitor(t, srv, objID, 5, 3)

	c.Send(encodeRequestAutoSoulShot(6645, 1))
	assertExAutoSoulShot(t, c.Read(), 6645, true)
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageUseOfItemWillBeAuto, 6645)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageEnabledSoulshot)
	assertMagicSkillUseSelf(t, c.Read(), objID, 2150, 1, 0, 0)
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "PetUsesS1")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessagePetUsesS1 {
		t.Fatalf("message id = %d, want PetUsesS1 (%d)", id, serverpackets.SystemMessagePetUsesS1)
	}
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "beast charge MagicSkillUse")
	r := wire.NewReader(frame[1:])
	if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != servitor.ObjectID() || target != caster || skill != 2033 {
		t.Fatalf("charge MagicSkillUse = %d/%d skill %d, want servitor-cast self 2033", caster, target, skill)
	}
	drainUntilQuiet(t, c)
	if got := carriedCount(t, srv, objID, 1463); got != 9 {
		t.Fatalf("soulshots after the owner charge = %d, want 9", got)
	}
	if got := carriedCount(t, srv, objID, 6645); got != 5 {
		t.Fatalf("beast soulshots after the servitor charge = %d, want 5", got)
	}
}
