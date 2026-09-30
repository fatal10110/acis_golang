package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// midSwingPickup is a player swinging bare-handed at the fixture monster
// with 40 adena lying at its feet.
type midSwingPickup struct {
	srv       *gameservertest.Server
	c         *scriptedClient
	objID     int32
	groundID  int32
	hostileID int32
}

// bootMidSwingPickup drops adena back units behind the player, attacks the
// adjacent fixture monster, and returns once the first swing's Attack is
// out, with that swing in flight.
func bootMidSwingPickup(t *testing.T, back int) midSwingPickup {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)
	groundID := dropAdenaBehind(t, c, adena, back)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	readUntil(t, c, serverpackets.OpcodeAttack, "first swing")
	return midSwingPickup{srv: srv, c: c, objID: objID, groundID: groundID, hostileID: hostile.ObjectID()}
}

// assertNoSwingBy fails when attackerID starts a swing within d. The
// monster's own swings back do not count.
func assertNoSwingBy(t *testing.T, c *scriptedClient, attackerID int32, d time.Duration, what string) {
	t.Helper()
	for end := c.Now().Add(d); c.Now().Before(end); {
		frame := c.ReadWithTimeout(end.Sub(c.Now()))
		if frame == nil {
			return
		}
		if frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == attackerID {
			t.Fatalf("player swung again %s", what)
		}
	}
}

// TestMidSwingPickupReplacesTheAttack pins PlayableAI.tryToPickUp
// (PlayableAI.java:411-428) for a swing in flight: the click is answered
// ActionFailed and kept as the next intention. When the swing ends,
// PlayableAI.onEvtFinishedAttack (PlayableAI.java:66-76) runs it in place of
// the ATTACK intention, and thinkPickUp (PlayableAI.java:199-234) ends
// idle, so the item is picked up and no further swing follows. An item out
// of reach is walked to first; the attack does not take the walk back to
// chase its target.
func TestMidSwingPickupReplacesTheAttack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		back int
	}{
		{name: "in reach", back: 0},
		{name: "walked to", back: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := bootMidSwingPickup(t, tc.back)

			s.c.Send(encodeAction(s.groundID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			assertFrameOpcode(t, s.c.Read(), serverpackets.OpcodeActionFailed, "queued pickup ActionFailed")
			for i := 0; ; i++ {
				frame := s.c.ReadWithTimeout(3 * time.Second)
				if frame == nil || i == 100 {
					t.Fatal("the queued pickup never ran")
				}
				if frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == s.objID {
					t.Fatal("the player swung again before the queued pickup ran")
				}
				if frame[0] == serverpackets.OpcodeGetItem {
					break
				}
			}
			assertNoSwingBy(t, s.c, s.objID, 3*time.Second, "after the queued pickup")
		})
	}
}

// TestMidSwingAttackReplacesQueuedPickup: the player has one next-intention
// slot, so an attack requested mid-swing after a queued pickup replaces it
// (PlayableAI.tryToAttack stores it with getNextIntention().updateAsAttack).
// The swing's end swings again and the item stays on the ground.
func TestMidSwingAttackReplacesQueuedPickup(t *testing.T) {
	t.Parallel()
	s := bootMidSwingPickup(t, 0)
	x, y, z := int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z)

	s.c.Send(encodeAction(s.groundID, x, y, z, false))
	assertFrameOpcode(t, s.c.Read(), serverpackets.OpcodeActionFailed, "queued pickup ActionFailed")
	s.c.Send(encodeAction(s.hostileID, x, y, z, false))

	swung := false
	for end := s.c.Now().Add(3 * time.Second); s.c.Now().Before(end); {
		frame := s.c.ReadWithTimeout(end.Sub(s.c.Now()))
		if frame == nil {
			break
		}
		switch {
		case frame[0] == serverpackets.OpcodeGetItem:
			t.Fatal("the pickup replaced by the attack still ran")
		case frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == s.objID:
			swung = true
		}
	}
	if !swung {
		t.Fatal("the attack queued behind the swing never swung")
	}
}

// TestMidSwingPickupOfVanishedItemEndsTheAttack: the queued pickup still
// replaces the ATTACK intention when its item left the ground before the
// swing ended. PlayableAI.thinkPickUp (PlayableAI.java:199-214) releases the
// click with ActionFailed and, the target lost, goes idle: nothing is picked
// up and no further swing follows.
func TestMidSwingPickupOfVanishedItemEndsTheAttack(t *testing.T) {
	t.Parallel()
	s := bootMidSwingPickup(t, 0)

	s.c.Send(encodeAction(s.groundID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, s.c.Read(), serverpackets.OpcodeActionFailed, "queued pickup ActionFailed")
	s.srv.DespawnGroundItem(t, s.groundID)

	deleted, released := false, false
	for end := s.c.Now().Add(3 * time.Second); s.c.Now().Before(end); {
		frame := s.c.ReadWithTimeout(end.Sub(s.c.Now()))
		if frame == nil {
			break
		}
		switch {
		case frame[0] == serverpackets.OpcodeDeleteObject && wireReader(frame[1:]).ReadInt32() == s.groundID:
			deleted = true
		case frame[0] == serverpackets.OpcodeActionFailed && deleted:
			released = true
		case frame[0] == serverpackets.OpcodeGetItem:
			t.Fatal("a vanished item was picked up")
		case frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == s.objID:
			t.Fatal("the player swung again after its queued pickup's item vanished")
		}
	}
	if !deleted {
		t.Fatal("the vanished item's DeleteObject never arrived")
	}
	if !released {
		t.Fatal("the swing's end never released the queued pickup with ActionFailed")
	}
}
