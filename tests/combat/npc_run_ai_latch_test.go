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

// latchAtkSpd is fast enough that a swing finishes before its hit animation
// timer, so the swing's finish closes the window and its runAI promotes.
const latchAtkSpd = 2000

// wanderThenStop has the monster idle, take one wander step and arrive, so
// its last executed desire is a wander and its queue is empty again.
func wanderThenStop(t *testing.T, _ *gameservertest.Server, hostile *npc.Hostile) {
	t.Helper()
	tickThinkWander(t, hostile)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the wander cycle = %v, want %v", got, ai.IntentionWander)
	}
	hostile.Move().CancelMove()
	hostile.AI().Arrived()
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the wander arrival = %v, want %v kept", got, ai.IntentionWander)
	}
	if got := hostile.AI().Desires().Len(); got != 0 {
		t.Fatalf("queued desires after the wander arrival = %d, want 0", got)
	}
}

// readAttackWithout reads up to the monster's next Attack and fails if a
// frame with the forbidden opcode comes first.
func readAttackWithout(t *testing.T, srv *gameservertest.Server, forbidden byte, what string) {
	t.Helper()
	for _, frame := range readUntil(t, srv.Client, serverpackets.OpcodeAttack, what) {
		if frame[0] == forbidden {
			t.Fatalf("opcode %#x before %s", forbidden, what)
		}
	}
}

// assertNoSwingFor lets d pass and fails on any Attack broadcast in it.
func assertNoSwingFor(t *testing.T, srv *gameservertest.Server, d time.Duration) {
	t.Helper()
	srv.Advance(t, d)
	for {
		frame := srv.Client.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return
		}
		if frame[0] == serverpackets.OpcodeAttack {
			t.Fatal("Attack after the latched swing, want the monster idle")
		}
	}
}

// TestLatchedFirstAttackSwingsOnceAfterDesireLost pins NpcAI.runAI's
// _nextDesire latch: a monster whose first attack after wandering loses its
// desire still swings at the latched target on the next runAI, the one its
// first swing's finish runs, instead of idling. The runAI after that idles.
func TestLatchedFirstAttackSwingsOnceAfterDesireLost(t *testing.T) {
	t.Parallel()
	srv, hostile, _, attackTime := startHostileAttackAfter(t, latchAtkSpd, 0, wanderThenStop)
	decayAttackDesire(t, hostile)

	srv.Advance(t, attackTime+10*time.Millisecond)
	readAttackWithout(t, srv, serverpackets.OpcodeChangeMoveType, "latched second swing")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the latched runAI = %v, want %v", got, ai.IntentionAttack)
	}

	intentionAfter(t, srv, hostile, attackTime, "once the latched swing finished, want idle", intentionIs(ai.IntentionIdle))
	assertNoSwingFor(t, srv, attackTime)
}

// TestHeavierDesireWaitsOneRunAIBehindLatchedAttack pins the latch's
// precedence: a heavier MOVE_TO queued right after a monster's first attack
// after wandering waits while the latched attack swings once more, and the
// runAI after that takes it up.
func TestHeavierDesireWaitsOneRunAIBehindLatchedAttack(t *testing.T) {
	t.Parallel()
	srv, hostile, _, attackTime := startHostileAttackAfter(t, latchAtkSpd, 0, wanderThenStop)
	decayAttackDesire(t, hostile)
	x, y, z := hostile.Position()
	if !hostile.AI().AddMoveToDesire(location.Location{X: x + 200, Y: y, Z: z}, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}

	srv.Advance(t, attackTime+10*time.Millisecond)
	readAttackWithout(t, srv, serverpackets.OpcodeMoveToLocation, "latched second swing")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the latched runAI = %v, want %v", got, ai.IntentionAttack)
	}

	intentionAfter(t, srv, hostile, attackTime, "once the latched swing finished, want move_to", intentionIs(ai.IntentionMoveTo))
	readUntil(t, srv.Client, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}
