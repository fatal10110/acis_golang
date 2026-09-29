package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// midSwingSwordID is the shared catalog's one-handed sword.
const midSwingSwordID int32 = 30

// TestWeaponUseItemMidSwingWaitsForTheSwing pins PlayableAI.tryToUseItem
// (PlayableAI.java:466-481) for a swing in flight: UseItem on a weapon is
// stored as the next intention, so the paperdoll keeps the weapon the swing
// started with until the swing ends. Then PlayerAI.thinkUseItem
// (PlayerAI.java:505-519) equips it and resumes the ATTACK intention that
// was current, which swings again.
func TestWeaponUseItemMidSwingWaitsForTheSwing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, midSwingSwordID, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	readUntil(t, c, serverpackets.OpcodeAttack, "first swing")

	c.Send(encodeUseItem(sword, false))
	srv.Settle(t)
	if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand); worn != nil {
		t.Fatalf("right hand mid-swing = %v, want the swing's empty hand kept", worn)
	}

	sawEquip := false
	for i := 0; i < 100 && !sawEquip; i++ {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatal("the queued sword was never equipped")
		}
		switch {
		case frame[0] == serverpackets.OpcodeAttack:
			t.Fatal("a swing started before the queued sword went on")
		case frame[0] == serverpackets.OpcodeSystemMessage && wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageS1Equipped:
			sawEquip = true
		}
	}
	if worn := srv.PlayerInventory(t, objID).ItemAt(itemcontainer.RHand); worn == nil || worn.ObjectID != sword {
		t.Fatalf("right hand after the swing = %v, want the sword", worn)
	}
	attack := readUntil(t, c, serverpackets.OpcodeAttack, "the resumed attack")[0]
	if got := wireReader(attack[1:]).ReadInt32(); got != objID {
		t.Fatalf("resumed Attack attacker = %d, want %d", got, objID)
	}
}

// TestWeaponUseItemMidBowShotRunsAtTheShotEnd: a toggle queued behind a bow
// shot runs when the shot ends, as any intention queued there does
// (PlayableAI.onEvtFinishedAttack). The sword replaces the bow, which takes
// its arrows out of the left hand (BowRodListener.onUnequip).
func TestWeaponUseItemMidBowShotRunsAtTheShotEnd(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a bow shot open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	bow := srv.GiveItem(t, objID, bowTemplateID, 1)
	srv.GiveItem(t, objID, woodenArrowTemplateID, bowArrowStack)
	sword := srv.GiveItem(t, objID, midSwingSwordID, 1)
	startInWorld(t, c)
	equipAndFlush(t, srv, c, bow)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	readUntil(t, c, serverpackets.OpcodeAttack, "first bow shot")

	c.Send(encodeUseItem(sword, false))
	srv.Settle(t)
	inv := srv.PlayerInventory(t, objID)
	if worn := inv.ItemAt(itemcontainer.RHand); worn == nil || worn.ObjectID != bow {
		t.Fatalf("right hand mid-shot = %v, want the bow kept", worn)
	}

	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("the queued sword was never equipped")
		}
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatal("the queued sword was never equipped")
		}
		if frame[0] == serverpackets.OpcodeSystemMessage && wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageS1Equipped {
			break
		}
	}
	if worn := inv.ItemAt(itemcontainer.RHand); worn == nil || worn.ObjectID != sword {
		t.Fatalf("right hand after the shot = %v, want the sword", worn)
	}
	if worn := inv.ItemAt(itemcontainer.LHand); worn != nil {
		t.Fatalf("left hand after the bow came off = %v, want the arrows gone", worn)
	}
}
