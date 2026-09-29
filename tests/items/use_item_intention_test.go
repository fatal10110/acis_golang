package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: UseItem.java:145-155 hands a weapon or shield to
// PlayableAI.tryToUseItem (PlayableAI.java:466-481): denyAiAction answers
// ActionFailed; a swing, cast, sit-down or stand-up in flight keeps the
// toggle as the next intention; PlayerAI.thinkUseItem (PlayerAI.java:505-519)
// then runs useEquippableItem and resumes the previous intention unless it
// was a cast.

// swordID is the shared catalog's one-handed sword.
const swordID int32 = 30

// readUntilEquipped reads frames until S1_EQUIPPED naming templateID and
// returns every frame read before it.
func readUntilEquipped(t *testing.T, c *testsupport.ScriptedClient, templateID int32) [][]byte {
	t.Helper()
	var before [][]byte
	for range 100 {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeSystemMessage {
			r := wire.NewReader(frame[1:])
			if r.ReadInt32() == serverpackets.SystemMessageS1Equipped {
				assertSystemMessageItem(t, frame, serverpackets.SystemMessageS1Equipped, templateID)
				return before
			}
		}
		before = append(before, frame)
	}
	t.Fatalf("no S1_EQUIPPED for %d within 100 frames", templateID)
	return nil
}

// assertRightHandEmpty requires objID's right hand to hold nothing yet.
func assertRightHandEmpty(t *testing.T, srv *gameservertest.Server, objID int32, what string) {
	t.Helper()
	srv.Settle(t)
	if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand); worn != nil {
		t.Fatalf("right hand %s = %v, want empty", what, worn)
	}
}

// TestWeaponUseItemWaitsOutCast: UseItem on a weapon mid-cast leaves the
// paperdoll alone until the cast ends, then equips it (S1_EQUIPPED, then
// UserInfo) after the cast's MagicSkillLaunched.
func TestWeaponUseItemWaitsOutCast(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(masteryItemSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 1060, 5)
	sword := srv.GiveItem(t, objID, swordID, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(potion, false))
	for frame := c.Read(); frame[0] != serverpackets.OpcodeMagicSkillUse; frame = c.Read() {
	}
	c.Send(encodeUseItem(sword, false))
	assertRightHandEmpty(t, srv, objID, "mid-cast")
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("the potion cast ended before the queued toggle was checked")
	}

	launched := false
	for _, frame := range readUntilEquipped(t, c, swordID) {
		if frame[0] == serverpackets.OpcodeMagicSkillLaunched {
			launched = true
		}
		if frame[0] == serverpackets.OpcodeUserInfo {
			t.Fatal("UserInfo refresh before the queued sword went on")
		}
	}
	if !launched {
		t.Fatal("the sword went on before the cast launched")
	}
	drainUntilQuiet(t, c)
	if e := inventoryUpdateAfterTick(t, srv, c)[sword]; e.equipped != 1 {
		t.Fatalf("sword InventoryUpdate entry = %+v, want equipped", e)
	}
}

// TestWeaponUseItemWaitsOutStandUp: UseItem on a weapon during a stand-up
// runs once STOOD_UP fires. The resumed STAND intention finds the player
// already up and answers ActionFailed after the equip refresh.
func TestWeaponUseItemWaitsOutStandUp(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("holding a stand-up open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, swordID, 1)
	startInWorld(t, c)
	sitAndSettle(t, srv)

	changePosture(t, c, true)
	standAt := c.Now()
	c.Send(encodeUseItem(sword, false))
	assertRightHandEmpty(t, srv, objID, "mid-stand-up")

	readUntilEquipped(t, c, swordID)
	if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
		t.Fatalf("queued sword went on %v after the stand-up, want no earlier than %v", elapsed, sitStandDelay)
	}
	sawUserInfo := false
	for range 20 {
		frame := c.Read()
		switch frame[0] {
		case serverpackets.OpcodeUserInfo:
			sawUserInfo = true
		case serverpackets.OpcodeActionFailed:
			if !sawUserInfo {
				t.Fatal("ActionFailed ahead of the equip UserInfo")
			}
			if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand); worn == nil || worn.ObjectID != sword {
				t.Fatalf("right hand after the stand-up = %v, want the sword", worn)
			}
			return
		}
	}
	t.Fatal("no ActionFailed closing the resumed stand intention")
}

// TestWeaponUseItemWhileTeleportingAnswersActionFailed: a player whose AI
// actions are denied (teleporting) is answered ActionFailed alone and keeps
// the paperdoll as it is.
func TestWeaponUseItemWhileTeleportingAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, swordID, 1)
	startInWorld(t, c)
	if !onlineLivePlayer(t, srv, objID).SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}

	c.Send(encodeUseItem(sword, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "denied UseItem")
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the denied UseItem")
	assertRightHandEmpty(t, srv, objID, "after the denied UseItem")
}
