package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFakeDeathUpProtectionFollowsTheConfiguredGrace pins
// players.properties PlayerFakeDeathUpProtection: once a Fake Death ends,
// a hostile NPC takes the player for an automatic target only after that
// many seconds. A warrior holding its ground (MovingAttack 0) wants to
// attack its attacker only when it may attack it on its own, so its answer
// to a hit tells whether the grace still runs. With no grace configured it
// wants to attack at once.
func TestFakeDeathUpProtectionFollowsTheConfiguredGrace(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		grace time.Duration
	}{
		{"ten seconds", 10 * time.Second},
		{"none", 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := warriorTemplate(behaviorWarriorID, npc.AIParams{"MovingAttack": "0"})
			tmpl.AggroRange = 400
			w := bootBehaviorsWith(t, []gameservertest.Option{gameservertest.WithFakeDeathUpProtection(row.grace)}, tmpl)
			if !w.srv.DrivesClock() {
				t.Skip("placing hits on either side of the grace needs the driven clock")
			}
			h := w.spawn(t, behaviorWarriorID, 100, 0)
			w.endFakeDeath(t)

			w.hit(t, h, 10)
			if _, ok := attackOn(h, w.player); ok != (row.grace == 0) {
				t.Fatalf("attack desire on the character right after the Fake Death = %v, want %v", ok, row.grace == 0)
			}
			if row.grace == 0 {
				return
			}
			w.srv.Advance(t, row.grace-10*time.Millisecond)
			w.hit(t, h, 10)
			if _, ok := attackOn(h, w.player); ok {
				t.Fatal("attack desire on the character just short of the grace, want none")
			}
			w.srv.Advance(t, 10*time.Millisecond)
			w.hit(t, h, 10)
			if _, ok := attackOn(h, w.player); !ok {
				t.Fatal("no attack desire on the character once the grace ran out")
			}
		})
	}
}

// endFakeDeath starts the character's post-Fake-Death grace, as the end of
// its Fake Death does, on its queue.
func (w *warriorWorld) endFakeDeath(t *testing.T) {
	t.Helper()
	pc, ok := w.player.(interface{ MarkRecentFakeDeath() })
	if !ok {
		t.Fatalf("the character %T cannot end a Fake Death", w.player)
	}
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		pc.MarkRecentFakeDeath()
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}
