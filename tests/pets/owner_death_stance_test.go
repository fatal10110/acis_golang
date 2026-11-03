package pets

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// autoAttackStops returns the AutoAttackStop frames in frames for one of
// ids, in order, with their indexes, and the index of the first Die for
// ids[0] (-1 if none).
func autoAttackStops(t *testing.T, frames [][]byte, ids ...int32) (stops []int32, stopAt []int, die int) {
	t.Helper()
	die = -1
	for i, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeAutoAttackStop:
			id := mustObjectID(t, frame)
			for _, want := range ids {
				if id == want {
					stops = append(stops, id)
					stopAt = append(stopAt, i)
				}
			}
		case serverpackets.OpcodeDie:
			if die < 0 && mustObjectID(t, frame) == ids[0] {
				die = i
			}
		}
	}
	return stops, stopAt, die
}

// TestOwnerDeathEndsOnlyTheOwnersStance pins the owner-death side of a
// shared stance: an owner killed in attack stance leaves it with its own
// AutoAttackStop right after its Die, while its summon gets none, neither
// then nor when the old stance window would have ended, since only that
// window's expiry ends the summon's stance with its owner's.
func TestOwnerDeathEndsOnlyTheOwnersStance(t *testing.T) {
	t.Parallel()
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
	drainUntilQuiet(t, h.client)

	onOwnerQueue(t, h, func(pc *player.Character) { pc.Kill(nil) })
	srv.Settle(t)
	stops, stopAt, die := autoAttackStops(t, drainFrames(t, h.client), ownerID, pet.ObjectID())
	if die < 0 {
		t.Fatal("the owner never received its own Die")
	}
	if len(stops) != 1 || stops[0] != ownerID || stopAt[0] != die+1 {
		t.Fatalf("AutoAttackStop ids %v at frames %v, want only the owner's %d right after Die at %d (summon %d)",
			stops, stopAt, ownerID, die, pet.ObjectID())
	}

	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	srv.Settle(t)
	if stops, _, _ := autoAttackStops(t, drainFrames(t, h.client), ownerID, pet.ObjectID()); len(stops) != 0 {
		t.Fatalf("old stance expiry sent AutoAttackStop for %v, want none (owner %d, summon %d)", stops, ownerID, pet.ObjectID())
	}
}
