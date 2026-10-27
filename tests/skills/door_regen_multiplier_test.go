package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/vitals"
)

// TestDamagedDoorRegenTakesHpRegenMultiplier: a door is never raid
// related, so its base HP regeneration (1.5) takes players.properties'
// HpRegenMultiplier, as CreatureStatus.getRegenHp multiplies it before the
// stat. At 4 each five-minute tick gives max(1, 1.5*4) = 6 HP (the ticks are
// the reference's own, internal/testsupport/vitals).
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
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(t, gate.ObjectID(), 590))

	// Each tick of the reference's door regeneration at that multiplier.
	for i, tick := range vitals.DoorRegenTicks(t, 4, 590) {
		srv.AdvanceDoorRegen(t, vitals.DoorRegenPeriod(t))
		assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(t, gate.ObjectID(), tick.HP))
		if gate.CurrentHP() != tick.HP || srv.DoorRegenerating(t, hpDoorID) != tick.Active {
			t.Fatalf("after tick %d: HP %v, regenerating %v; want HP %v, regenerating %v",
				i+1, gate.CurrentHP(), srv.DoorRegenerating(t, hpDoorID), tick.HP, tick.Active)
		}
	}
	if gate.CurrentHP() != hpDoorMaxHP || srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("full door HP %v, regenerating %v; want %d, stopped", gate.CurrentHP(), srv.DoorRegenerating(t, hpDoorID), hpDoorMaxHP)
	}
}
