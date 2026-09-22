package character

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// panicOnStanceCheck fails the exit guard's stance read once armed, so the
// task RequestRestart posts to the player's queue panics part-way. Add and
// Remove stay no-ops: only the guard's read is the failure point.
type panicOnStanceCheck struct{ armed atomic.Bool }

func (p *panicOnStanceCheck) Add(task.AttackStanceActor) {}

func (p *panicOnStanceCheck) Remove(task.AttackStanceActor) bool { return false }

func (p *panicOnStanceCheck) InAttackStance(task.AttackStanceActor) bool {
	if p.armed.Load() {
		panic("attack-stance read panic")
	}
	return false
}

// TestPanicInQueuedRestartHandlerDropsSession covers the one dispatch site
// that clears live: RequestRestart. Everywhere else a panicking task is
// caught by the guard at the top of the read loop, which reads live. Here the
// case nils that pointer on its way to character selection, so a discarded
// onLive result would leave the guard permanently dead for the session — the
// connection would be told the restart succeeded while the character stayed
// registered in the world with its queue open and its saves unawaited, and
// Handle's deferred detach would be skipped too.
//
// The character must be gone from world state afterwards: that teardown can
// only come from the deferred detach, which runs precisely because live was
// left in place.
func TestPanicInQueuedRestartHandlerDropsSession(t *testing.T) {
	if os.Getenv(gameservertest.SimExecutorEnv) == "inline" {
		// sim.Inline deliberately does not recover, so a panicking task
		// takes the harness pump goroutine with it. The policy under test
		// is a production (pool) one.
		t.Skip("sim.Inline does not recover task panics")
	}
	stance := &panicOnStanceCheck{}
	srv := gameservertest.Boot(t,
		gameservertest.WithAttackStanceTracker(stance),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	enterWorld(t, c)
	objID := srv.SoleObjectID(t)

	stance.armed.Store(true)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))

	if !c.AwaitClose(5 * time.Second) {
		t.Fatal("session stayed open after the queued restart handler panicked")
	}
	srv.FlushPersistence(t)
	if _, ok := srv.State.Player(objID); ok {
		t.Fatalf("character %d left registered in the world after the panic", objID)
	}
}
