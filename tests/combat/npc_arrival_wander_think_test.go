package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestWanderArrivalThenSleepExitRunsWanderTimer pins NpcAI.onEvtArrived
// keeping WANDER current and the sleep exit's THINK running
// AttackableAI.thinkWander on it: the wander timer starts at the THINK, so
// the next random walk comes one wander timer after the sleep ends, not
// one timer after the next wander promotion.
func TestWanderArrivalThenSleepExitRunsWanderTimer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	// Every roll walks, so the step taken is the timer's alone.
	hostile.AI().SetRandomWalkRate(100)

	tickThinkWander(t, hostile)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the wander cycle = %v, want %v", got, ai.IntentionWander)
	}
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "MoveToLocation for the first wander step")

	for i := 0; hostile.Move().Moving(); i++ {
		if i >= int(10*time.Second/move.PositionUpdateInterval) {
			t.Fatal("wander step never arrived")
		}
		srv.TickPositions()
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the wander arrival = %v, want %v kept", got, ai.IntentionWander)
	}
	if hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionWander}) {
		t.Fatal("wander desire still queued after the arrival")
	}
	drainUntilQuiet(t, c)

	landEffect(t, hostile, "Sleep")
	onHostileQueue(t, hostile, func() { hostile.EffectList().StopByType(effect.TypeSleep) })
	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after the sleep ended")
	}
	drainUntilQuiet(t, c)

	wanderTimer := 5 * time.Second
	srv.Advance(t, 2*time.Second)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("idle TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the empty-queue cycle = %v, want %v", got, ai.IntentionIdle)
	}
	srv.Advance(t, time.Second)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the promotion = %v, want %v", got, ai.IntentionWander)
	}
	assertNoWanderWalk(t, c, "the wander promotion, want the sleep exit's timer still running")

	srv.Advance(t, wanderTimer-3*time.Second)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("timer TickThink() error: %v", err)
	}
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "MoveToLocation one wander timer after the sleep exit")
	if !hostile.IsMoving() {
		t.Fatal("IsMoving() = false when the sleep exit's wander timer ran out, want the next random walk")
	}
}

// assertNoWanderWalk fails on any MoveToLocation read before the client
// goes quiet.
func assertNoWanderWalk(t *testing.T, c *scriptedClient, after string) {
	t.Helper()
	for frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(300 * time.Millisecond) {
		if frame[0] == serverpackets.OpcodeMoveToLocation {
			t.Fatalf("MoveToLocation after %s", after)
		}
	}
}
