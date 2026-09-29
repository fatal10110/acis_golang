package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
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

// TestQueuedWeaponDestroyedMidSwingIsDroppedWithTheAttack pins the early
// return of PlayerAI.thinkUseItem (PlayerAI.java:509-511): a sword queued
// mid-swing and destroyed before the swing ends is not equipped when the
// swing ends, and the ATTACK intention it replaced does not resume, so no
// further swing starts.
func TestQueuedWeaponDestroyedMidSwingIsDroppedWithTheAttack(t *testing.T) {
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
	c.Send(encodeRequestDestroyItem(sword, 1))
	srv.Settle(t)
	inv := srv.PlayerInventory(t, objID)
	if inv.ItemByObjectID(sword) != nil {
		t.Fatal("the queued sword survived RequestDestroyItem mid-swing")
	}

	// Three seconds of clock let the swing end; the resumed attack would
	// swing again well inside them. The hostile's own swings back do not
	// count.
	for end := c.Now().Add(3 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(end.Sub(c.Now()))
		if frame == nil {
			break
		}
		switch {
		case frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == objID:
			t.Fatal("the attack resumed after the queued sword was destroyed")
		case frame[0] == serverpackets.OpcodeSystemMessage && wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageS1Equipped:
			t.Fatal("S1_EQUIPPED sent for the destroyed sword")
		}
	}
	if worn := inv.ItemAt(itemcontainer.RHand); worn != nil {
		t.Fatalf("right hand after the swing = %v, want empty", worn)
	}
}

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
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
