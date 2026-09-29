package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// testReviver records the refusals its resurrection offers get.
type testReviver struct{ refusals []event.ReviveRefusal }

func (*testReviver) CharacterName() string { return "Reviver" }

func (r *testReviver) NotifyReviveRefused(reason event.ReviveRefusal) {
	r.refusals = append(r.refusals, reason)
}

// TestReviveRefusedWhileTeleporting matches Playable.doRevive's
// isTeleporting gate: a dead player in the middle of a teleport is not
// revived, stays at 0 HP, and keeps its open resurrection offer. The same
// player revives once the teleport ends.
func TestReviveRefusedWhileTeleporting(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.MarkDead()
	reviver := &testReviver{}
	c.ReviveRequest(reviver, 50, false)
	if !c.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}

	if c.Revive() {
		t.Fatal("Revive() = true while teleporting")
	}
	if !c.Dead() {
		t.Fatal("Dead() = false after a refused revive")
	}
	if hp := c.CurrentHP(); hp != 0 {
		t.Fatalf("CurrentHP() = %d after a refused revive, want 0", hp)
	}
	c.ReviveRequest(reviver, 50, false)
	if len(reviver.refusals) != 1 || reviver.refusals[0] != event.ReviveAlreadyProposed {
		t.Fatalf("second offer refusals = %v, want [ReviveAlreadyProposed]: the open offer was dropped", reviver.refusals)
	}

	c.SetTeleporting(false)
	if !c.Revive() {
		t.Fatal("Revive() = false once the teleport ended")
	}
	if c.Dead() {
		t.Fatal("Dead() = true after the revive")
	}
}
