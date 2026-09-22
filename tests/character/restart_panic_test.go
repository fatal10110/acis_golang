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
// task RequestRestart posts to the player's queue dies before
// detachLivePlayer is entered. Add and Remove stay no-ops: only the guard's
// read is the failure point.
type panicOnStanceCheck struct{ armed atomic.Bool }

func (p *panicOnStanceCheck) Add(task.AttackStanceActor) {}

func (p *panicOnStanceCheck) Remove(task.AttackStanceActor) bool { return false }

func (p *panicOnStanceCheck) InAttackStance(task.AttackStanceActor) bool {
	if p.armed.Load() {
		panic("attack-stance read panic")
	}
	return false
}

// panicOnStanceRemove fails the first stance removal detachLivePlayer
// reaches, so the teardown dies part-way instead of before it starts. Later
// removals succeed, so the detach that resumes behind the panic runs to the
// end; the call count is what proves it reached the tracker again.
type panicOnStanceRemove struct{ calls atomic.Int32 }

func (p *panicOnStanceRemove) Add(task.AttackStanceActor) {}

func (p *panicOnStanceRemove) InAttackStance(task.AttackStanceActor) bool { return false }

func (p *panicOnStanceRemove) Remove(task.AttackStanceActor) bool {
	if p.calls.Add(1) == 1 {
		panic("attack-stance remove panic")
	}
	return true
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
	skipWithoutTaskRecovery(t)
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

// TestPanicInsideDetachCompletesTeardown drives the panic into the middle of
// detachLivePlayer rather than ahead of it, which is the case the policy's
// "the deferred detach finishes the teardown the panicking task abandoned"
// actually rests on: the first pass dies inside live.Stop(), before the
// character save is enqueued and before the world removal, so every part of
// the teardown that lands has to come from the re-entrant second pass.
//
// Damage applied before the request is the observable for it. The save is
// enqueued after live.Stop() (lifecycle.go), so the panicking pass never
// reached it; finding the reduced HP persisted proves the re-entrant detach
// ran that far and not merely that the session closed.
func TestPanicInsideDetachCompletesTeardown(t *testing.T) {
	skipWithoutTaskRecovery(t)
	stance := &panicOnStanceRemove{}
	srv := gameservertest.Boot(t,
		gameservertest.WithAttackStanceTracker(stance),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	enterWorld(t, c)
	objID := srv.SoleObjectID(t)

	const damage = 10
	wantHP := srv.PlayerCurrentHP(t, objID) - damage
	srv.DamagePlayerHP(t, objID, damage)
	// The stop path only reaches the tracker for a player in combat.
	srv.SetPlayerInCombat(t, objID, true)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))

	if !c.AwaitClose(5 * time.Second) {
		t.Fatal("session stayed open after detachLivePlayer panicked part-way")
	}
	srv.FlushPersistence(t)
	// Two, not one. The first is live.Stop's stopLiveAutoAttack, which
	// panics; the second is detachLivePlayer's own unconditional removal.
	// stopLiveAutoAttack clears the in-combat flag before it reaches the
	// tracker, so the resumed pass returns early there — without the
	// unconditional removal the entry would survive the detach and, past
	// q.Close, could never be swept.
	if got := stance.calls.Load(); got != 2 {
		t.Fatalf("stance Remove calls = %d, want 2: the resumed detach must still drop the stance entry", got)
	}
	if _, ok := srv.State.Player(objID); ok {
		t.Fatalf("character %d left registered in the world after a panic inside detach", objID)
	}
	if ch := persistedCharacter(t, srv, objID); ch.CurrentHP() != wantHP {
		t.Fatalf("persisted HP = %d, want %d: the re-entrant detach never reached the character save", ch.CurrentHP(), wantHP)
	}
}

// skipWithoutTaskRecovery skips a panic-driving test under the inline
// executor: sim.Inline deliberately does not recover, so a panicking task
// takes the harness pump goroutine with it. The policy under test is a
// production (pool) one.
func skipWithoutTaskRecovery(t *testing.T) {
	t.Helper()
	if os.Getenv(gameservertest.SimExecutorEnv) == "inline" {
		t.Skip("sim.Inline does not recover task panics")
	}
}
