package task

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// reusedIDActor is an AI actor and a position updater at once, so one fake
// covers both registries keyed by object id.
type reusedIDActor struct {
	world.Presence
	id      int32
	thinks  int
	updates int
}

func (a *reusedIDActor) ObjectID() int32      { return a.id }
func (*reusedIDActor) Kind() actor.Kind       { return actor.KindNPC }
func (*reusedIDActor) Queue() *sim.Queue      { return testQueue }
func (*reusedIDActor) Tick()                  {}
func (a *reusedIDActor) TickThink() error     { a.thinks++; return nil }
func (a *reusedIDActor) PositionUpdate() bool { a.updates++; return true }

// TestAIRemoveKeepsReissuedIDHolder registers actor A under id N, then
// actor B under the same id (the id released by A and handed out again),
// and runs A's late Remove: B must stay registered and keep ticking.
func TestAIRemoveKeepsReissuedIDHolder(t *testing.T) {
	ai := NewAI(nil, zerolog.Nop())
	old := &reusedIDActor{id: 7}
	reissued := &reusedIDActor{id: 7}

	ai.Add(old)
	ai.Add(reissued)
	ai.Remove(old)

	if !ai.contains(7, reissued) {
		t.Fatal("late Remove of the old holder unregistered the actor now holding id 7")
	}
	if err := ai.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	testLoop.Run()
	if reissued.thinks != 1 || old.thinks != 0 {
		t.Fatalf("thinks after Tick: reissued %d old %d, want 1 and 0", reissued.thinks, old.thinks)
	}

	ai.Remove(reissued)
	if ai.contains(7, reissued) {
		t.Fatal("Remove of the current holder left it registered")
	}
	if err := ai.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	testLoop.Run()
	if reissued.thinks != 1 {
		t.Fatalf("reissued thinks after its own Remove = %d, want 1", reissued.thinks)
	}
}

// TestPositionUpdatesRemoveKeepsReissuedIDHolder is the same id-reuse case
// for movement correction, plus Contains answering for the actor itself
// rather than for whichever actor holds its id.
func TestPositionUpdatesRemoveKeepsReissuedIDHolder(t *testing.T) {
	positions := NewPositionUpdates(nil)
	old := &reusedIDActor{id: 9}
	reissued := &reusedIDActor{id: 9}

	positions.Add(old)
	positions.Add(reissued)
	if positions.Contains(old) {
		t.Fatal("Contains reports the old holder registered after its id was taken over")
	}
	positions.Remove(old)

	if !positions.Contains(reissued) {
		t.Fatal("late Remove of the old holder unregistered the actor now holding id 9")
	}
	positions.Tick()
	testLoop.Run()
	if reissued.updates != 1 || old.updates != 0 {
		t.Fatalf("updates after Tick: reissued %d old %d, want 1 and 0", reissued.updates, old.updates)
	}

	positions.Remove(reissued)
	if positions.Contains(reissued) {
		t.Fatal("Remove of the current holder left it registered")
	}
	positions.Tick()
	testLoop.Run()
	if reissued.updates != 1 {
		t.Fatalf("reissued updates after its own Remove = %d, want 1", reissued.updates)
	}
}
