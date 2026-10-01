package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wanderChainTimer is the idle wander desire's timer.
const wanderChainTimer = 5 * time.Second

// spawnWanderChainMonster boots a player near a monster that takes its
// first wander step and arrives, idles, then has its WANDER promoted again.
// That promotion follows a wander, so it starts the wander chain instead
// of walking. It returns the time of the promotion.
func spawnWanderChainMonster(t *testing.T, rate int) (*gameservertest.Server, *npc.Hostile, time.Time) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	hostile.AI().SetRandomWalkRate(rate)

	wanderThenStop(t, srv, hostile)
	drainUntilQuiet(t, c)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("idle TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the empty-queue cycle = %v, want %v", got, ai.IntentionIdle)
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	promoted := hostile.Now()
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the promotion = %v, want %v", got, ai.IntentionWander)
	}
	assertNoWanderWalk(t, c, "the wander promotion, want the chain started instead")
	return srv, hostile, promoted
}

// advanceTo lets the clock run up to at.
func advanceTo(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile, at time.Time) {
	t.Helper()
	if d := at.Sub(hostile.Now()); d > 0 {
		srv.Advance(t, d)
	}
}

// TestStunnedMonsterWanderHitEndsChain pins AttackableAI.thinkWander's
// task chain firing through a stun: the hit at the wander timer cannot
// walk a stunned monster and ends the chain. After the stun, the wander
// stays current with its desire queued, and runAI does not restart a
// current wander, so no cycle, event or timer walks it again. Once its
// wander desire is gone, the monster idles, and the next promotion starts
// a new chain that walks one wander timer later.
func TestStunnedMonsterWanderHitEndsChain(t *testing.T) {
	t.Parallel()
	srv, hostile, promoted := spawnWanderChainMonster(t, 100)
	c := srv.Client
	stun := landHeldEffect(t, hostile, "Stun")
	drainUntilQuiet(t, c)

	advanceTo(t, srv, hostile, promoted.Add(wanderChainTimer+time.Second))
	assertNoWanderWalk(t, c, "the hit while stunned, want the walk refused")
	removeHeldEffect(t, hostile, stun)

	for range 3 * int(wanderChainTimer/time.Second) {
		srv.Advance(t, time.Second)
		if err := hostile.TickThink(); err != nil {
			t.Fatalf("TickThink() after the stun error: %v", err)
		}
		if err := hostile.RunAI(); err != nil {
			t.Fatalf("RunAI() after the stun error: %v", err)
		}
	}
	assertNoWanderWalk(t, c, "the stun, want the ended chain to stay ended")
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true after the stun, want the monster standing")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the stun = %v, want %v kept", got, ai.IntentionWander)
	}
	if !hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionWander}) {
		t.Fatal("wander desire left the queue after the stun, want it kept")
	}

	hostile.AI().Desires().Remove(ai.IntentionWander, nil)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("idle TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() once the wander desire is gone = %v, want %v", got, ai.IntentionIdle)
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	restarted := hostile.Now()
	drainUntilQuiet(t, c)
	advanceTo(t, srv, hostile, restarted.Add(wanderChainTimer))
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "MoveToLocation one wander timer after the new chain started")
}

// TestStunnedMonsterWanderChainKeepsRolling pins the chain's firings as
// independent of desire selection and of the actor's control state: a
// miss during a stun reschedules the next firing one wander timer later,
// and that firing walks once the stun is gone with no AI cycle run.
func TestStunnedMonsterWanderChainKeepsRolling(t *testing.T) {
	t.Parallel()
	srv, hostile, promoted := spawnWanderChainMonster(t, 0)
	c := srv.Client
	stun := landHeldEffect(t, hostile, "Stun")
	drainUntilQuiet(t, c)

	advanceTo(t, srv, hostile, promoted.Add(wanderChainTimer+time.Second))
	removeHeldEffect(t, hostile, stun)
	hostile.AI().SetRandomWalkRate(100)
	assertNoWanderWalk(t, c, "the miss while stunned")

	second := promoted.Add(2 * wanderChainTimer)
	advanceTo(t, srv, hostile, second.Add(-time.Second))
	assertNoWanderWalk(t, c, "the stun, before the rescheduled firing")
	if !hostile.Now().Before(second) {
		t.Fatalf("clock %v already at the rescheduled firing %v", hostile.Now(), second)
	}

	advanceTo(t, srv, hostile, second)
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "MoveToLocation at the rescheduled firing")
	if !hostile.IsMoving() {
		t.Fatal("IsMoving() = false at the rescheduled firing, want the random walk")
	}
}
