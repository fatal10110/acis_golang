package items

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPanicInQueuedHandlerDropsSession covers the policy for a handler that
// panics on the player's queue: the pool recovers the panic so the worker
// survives, but the connection that was waiting for that task must not read
// on as if the handler had succeeded. The handler stopped part-way through
// its mutation and the client was told nothing, so the session ends exactly
// as a fatal decode error ends it — the character is saved and detached.
//
// The enchant roll is the vehicle only because it is an injectable step
// reached from inside a real mutating in-world handler, on a real queue.
// Damage applied before the request is the observable the detach save
// carries: nothing else writes current HP to the row, so finding it
// persisted proves detachLivePlayer ran and enqueued its saves.
func TestPanicInQueuedHandlerDropsSession(t *testing.T) {
	if os.Getenv(gameservertest.SimExecutorEnv) == "inline" {
		// sim.Inline deliberately does not recover, so a panicking task
		// takes the harness pump goroutine and the test process with it.
		// The policy under test is a production (pool) one.
		t.Skip("sim.Inline does not recover task panics")
	}
	srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { panic("enchant roll panic") }, 0, false, nil)
	c := srv.Client

	const damage = 10
	before := srv.PlayerCurrentHP(t, objID)
	srv.DamagePlayerHP(t, objID, damage)
	wantHP := before - damage

	openEnchantSelection(t, c, scroll, 955)
	c.Send(encodeRequestEnchantItem(weapon))

	if !c.AwaitClose(5 * time.Second) {
		t.Fatal("session stayed open after a queued handler panicked")
	}
	srv.FlushPersistence(t)

	if _, ok := srv.State.Player(objID); ok {
		t.Fatalf("world.Player(%d) still present after the panic disconnect", objID)
	}
	ch, err := srv.Chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load character: %v", err)
	}
	if ch.CurrentHP() != wantHP {
		t.Fatalf("persisted HP = %d, want %d: detach saves did not run", ch.CurrentHP(), wantHP)
	}
}
