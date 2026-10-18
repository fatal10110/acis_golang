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

// readUntilWithout reads up to the next frame with opcode want sent for
// objectID and returns it, failing if a frame with the forbidden opcode
// comes first.
func readUntilWithout(t *testing.T, srv *gameservertest.Server, objectID int32, want, forbidden byte, what string) []byte {
	t.Helper()
	for range 100 {
		frame := srv.Client.ReadWithTimeout(time.Second)
		switch {
		case frame == nil:
			t.Fatalf("%s never arrived", what)
		case frame[0] == forbidden:
			t.Fatalf("opcode %#x before %s", forbidden, what)
		case isOwnFrame(frame, want, objectID):
			return frame
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
	return nil
}

// readAttackWithout reads up to the monster's next Attack and fails if a
// frame with the forbidden opcode comes first.
func readAttackWithout(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile, forbidden byte, what string) {
	t.Helper()
	readUntilWithout(t, srv, hostile.ObjectID(), serverpackets.OpcodeAttack, forbidden, what)
}

// attackingAfterLatchedRunAI checks the monster still attacks once its
// latched second swing went out. Only a driven clock holds that moment: on
// the wall clock the first swing's hit animation can end before the check
// runs, and the runAI it brings already moved the monster on.
func attackingAfterLatchedRunAI(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile) {
	t.Helper()
	if !srv.DrivesClock() {
		return
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the latched runAI = %v, want %v", got, ai.IntentionAttack)
	}
}

// assertLatchedSwingThenIdle checks a monster whose attack desire was lost
// with its first swing latched: the next runAI swings once more without
// idling first, and the runAI after that idles with no swing in between.
// The idle shows no frame and lasts only until the next hit animation end
// takes up the idle wander it queued, so the wall clock, which cannot stop
// inside that window, checks the idle by that wander's MoveToLocation, the
// first frame only an idle leads to; the driven clock also reads the idle
// intention itself. No swing follows.
func assertLatchedSwingThenIdle(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile, attackTime time.Duration) {
	t.Helper()
	srv.Advance(t, attackTime+10*time.Millisecond)
	readAttackWithout(t, srv, hostile, serverpackets.OpcodeChangeMoveType, "latched second swing")
	attackingAfterLatchedRunAI(t, srv, hostile)

	srv.Advance(t, attackTime)
	if srv.DrivesClock() {
		if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
			t.Fatalf("CurrentIntention() once the latched swing finished = %v, want %v", got, ai.IntentionIdle)
		}
	}
	readUntilWithout(t, srv, hostile.ObjectID(), serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeAttack, "idle wander after the latched swing")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after the idle wander set off = %v, want %v", got, ai.IntentionWander)
	}
	assertNoSwingFor(t, srv, attackTime)
}

// assertNoSwingFor lets d pass and fails on any Attack broadcast in it.
func assertNoSwingFor(t *testing.T, srv *gameservertest.Server, d time.Duration) {
	t.Helper()
	srv.Advance(t, d)
	for _, frame := range srv.Client.ReadQueued() {
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

	assertLatchedSwingThenIdle(t, srv, hostile, attackTime)
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
	readAttackWithout(t, srv, hostile, serverpackets.OpcodeMoveToLocation, "latched second swing")
	attackingAfterLatchedRunAI(t, srv, hostile)

	intentionAfter(t, srv, hostile, attackTime, "once the latched swing finished, want move_to", intentionIs(ai.IntentionMoveTo))
	readUntil(t, srv.Client, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}
