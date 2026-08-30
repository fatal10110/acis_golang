package pets

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAttackStanceTimeoutBroadcastsSummonAutoAttackStop pins that a
// player's combat-stance timeout also stops the live summon's attack
// animation: known recipients get AutoAttackStop for the owner and for the
// summon, matching the reference timeout's summon broadcast.
func TestAttackStanceTimeoutBroadcastsSummonAutoAttackStop(t *testing.T) {
	var nowMS atomic.Int64
	srv := bootPets(t, gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }))
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	px, py, pz := srv.PlayerPosition(t, ownerID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainFrames(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeAutoAttackStart, "owner AutoAttackStart")

	h.client.Send(encodeMoveBackwardToLocation(int32(px-2000), int32(py+2000), int32(pz)))
	drainUntilQuiet(t, h.client)

	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}

	seen := map[int32]bool{}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (len(seen) < 2) {
		frame := h.client.ReadWithTimeout(200 * time.Millisecond)
		if frame == nil {
			continue
		}
		if frame[0] != serverpackets.OpcodeAutoAttackStop {
			continue
		}
		id := mustObjectID(t, frame)
		seen[id] = true
	}
	if !seen[ownerID] {
		t.Fatalf("timeout AutoAttackStop missing owner id %d; got %v", ownerID, seen)
	}
	if !seen[pet.ObjectID()] {
		t.Fatalf("timeout AutoAttackStop missing summon id %d; got %v", pet.ObjectID(), seen)
	}
}

func encodeMoveBackwardToLocation(targetX, targetY, targetZ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(targetX)
	w.WriteInt32(targetY)
	w.WriteInt32(targetZ)
	w.WriteInt32(10)
	w.WriteInt32(20)
	w.WriteInt32(30)
	w.WriteInt32(1)
	return w.Bytes()
}

func mustObjectID(t *testing.T, frame []byte) int32 {
	t.Helper()
	if len(frame) < 5 {
		t.Fatalf("AutoAttackStop too short: %x", frame)
	}
	return int32(frame[1]) | int32(frame[2])<<8 | int32(frame[3])<<16 | int32(frame[4])<<24
}
