package combat

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTeleportKeepsAttackStanceUntilItsTimeout: Creature.teleportTo
// (Creature.java:386-429) only runs abortAll(true) — move, attack and cast
// stop — and never reaches AttackStanceTaskManager. The stance ends only on
// its own expiry, which broadcasts AutoAttackStop
// (AttackStanceTaskManager.java:51), on death or on logout. A player who
// teleports mid-fight therefore gets no AutoAttackStop at the teleport, stays
// in the tracker and in combat, and sees AutoAttackStop once the stance
// times out.
func TestTeleportKeepsAttackStanceUntilItsTimeout(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, objID)
	srv.AdvanceUntil(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	awaitAutoAttackStart(t, c, objID)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	// Teleport mid-fight and complete it with the client's Appearing; every
	// frame up to the quiet after the arrival is checked.
	pc.TeleportTo(playerOrigin.X+300, playerOrigin.Y, playerOrigin.Z, 0)
	var frames [][]byte
	sawTeleport := false
	collect := func() {
		for i := 0; i < 200; i++ {
			frame := c.ReadWithTimeout(300 * time.Millisecond)
			if frame == nil {
				return
			}
			if frame[0] == serverpackets.OpcodeTeleportToLocation {
				sawTeleport = true
			}
			frames = append(frames, frame)
		}
		t.Fatal("client kept receiving frames after 200 reads")
	}
	collect()
	if !sawTeleport {
		t.Fatal("teleport sent no TeleportToLocation")
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	collect()
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeAutoAttackStop && wireReader(frame[1:]).ReadInt32() == objID {
			t.Fatal("teleport sent the player's AutoAttackStop; the stance must outlive it")
		}
	}
	if !srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("teleport removed the player from the attack-stance tracker")
	}
	if !pc.InCombat() {
		t.Fatal("teleport cleared the player's in-combat state")
	}
	// The teleport still aborted the attack: no further swing lands.
	afterTeleport := hostile.CurrentHP()
	srv.Advance(t, 1500*time.Millisecond)
	if got := hostile.CurrentHP(); got != afterTeleport {
		t.Fatalf("hostile HP = %d after the teleport, want frozen at %d (attack not aborted)", got, afterTeleport)
	}
	drainUntilQuiet(t, c)

	// The stance ends on its own timeout, with the player's AutoAttackStop.
	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	readSkippingCombat(t, c, serverpackets.OpcodeAutoAttackStop, objID, "timeout AutoAttackStop")
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("timeout left the player in the stance tracker")
	}
}
