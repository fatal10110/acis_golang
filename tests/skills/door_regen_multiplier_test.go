package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDamagedDoorRegenTakesHpRegenMultiplier: a door is never raid
// related, so its base HP regeneration (1.5) takes players.properties'
// HpRegenMultiplier, as CreatureStatus.getRegenHp multiplies it before the
// stat. At 4 each five-minute tick gives max(1, 1.5*4) = 6 HP: a door hit
// down to 590/600 shows 596 after one tick and is full after the second.
// MpRegenMultiplier and CpRegenMultiplier change nothing for a door.
func TestDamagedDoorRegenTakesHpRegenMultiplier(t *testing.T) {
	t.Parallel()
	srv, objID := bootUnlockOpts(t, []gameservertest.Option{gameservertest.WithRegenMultipliers(4, 7, 9)},
		unlockSkill(9000, "UNLOCK", unlockSkillLevel, 0), hpDoorTemplate())
	gate, ok := srv.WorldObjects.Door(hpDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	c := srv.Client
	srv.BeginDoorSiege(t, hpDoorID)

	srv.HitDoor(t, hpDoorID, objID, 10)
	// 590/600: 6 - ceil(5.9) = stage 0.
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 590))

	srv.AdvanceDoorRegen(t, 5*time.Minute)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 596))
	if gate.CurrentHP() != 596 {
		t.Fatalf("HP after one tick = %v, want 596", gate.CurrentHP())
	}
	// 596 + 6 caps at the maximum and stops the task.
	srv.AdvanceDoorRegen(t, 5*time.Minute)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, hpDoorMaxHP))
	if gate.CurrentHP() != hpDoorMaxHP || srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("full door HP %v, regenerating %v; want %d, stopped", gate.CurrentHP(), srv.DoorRegenerating(t, hpDoorID), hpDoorMaxHP)
	}
}
