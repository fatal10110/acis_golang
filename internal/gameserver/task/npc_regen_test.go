package task

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type countingRegenActor struct {
	id    int32
	ticks int
	on    func()
}

func (a *countingRegenActor) ObjectID() int32 { return a.id }
func (a *countingRegenActor) TickRegen() {
	a.ticks++
	if a.on != nil {
		a.on()
	}
}

// TestNPCRegenTickReusesScratchAcrossShrinkingPopulation is the regression
// case for reusing NPCRegen's scratch buffer (AppendObjects) across ticks:
// every remaining actor must still regenerate once the population shrinks,
// and a despawned actor must never be visited again — the failure mode a
// buffer that accumulates instead of truncating (or a stale tail the
// truncation forgets to clear) would produce.
func TestNPCRegenTickReusesScratchAcrossShrinkingPopulation(t *testing.T) {
	state := world.New()
	actors := make([]*countingRegenActor, 5)
	for i := range actors {
		actors[i] = &countingRegenActor{id: int32(i) + 1}
		state.AddObject(actors[i])
	}

	regen := NewNPCRegen(state)
	regen.Tick()
	for _, a := range actors {
		if a.ticks != 1 {
			t.Fatalf("actor %d ticks = %d after first Tick, want 1", a.id, a.ticks)
		}
	}

	state.RemoveObject(actors[2].id)
	state.RemoveObject(actors[4].id)
	regen.Tick()

	for i, a := range actors {
		want := 2
		if i == 2 || i == 4 {
			want = 1 // removed before the second Tick: must not gain another visit
		}
		if a.ticks != want {
			t.Fatalf("actor %d ticks = %d after shrinking Tick, want %d", a.id, a.ticks, want)
		}
	}
}

// TestNPCRegenTickLogsReentrantCall proves the tickGuard added alongside
// the scratch-buffer reuse actually fires: a second Tick call arriving
// while the first is still running (here, from inside a TickRegen hook,
// the same pattern AttackStance's own reentrant test uses) must be
// rejected and logged rather than appending into the in-flight scratch
// buffer from two call sites at once.
func TestNPCRegenTickLogsReentrantCall(t *testing.T) {
	state := world.New()
	regen := NewNPCRegen(state)
	var buf bytes.Buffer
	regen.log = zerolog.New(&buf)

	reentered := false
	actor := &countingRegenActor{id: 1}
	actor.on = func() {
		reentered = true
		regen.Tick()
	}
	state.AddObject(actor)

	regen.Tick()

	if !reentered {
		t.Fatal("reentrant Tick was never attempted")
	}
	if actor.ticks != 1 {
		t.Fatalf("actor ticks = %d, want 1 (reentrant Tick must not run TickRegen again)", actor.ticks)
	}
	if !strings.Contains(buf.String(), "NPCRegen.Tick") || !strings.Contains(buf.String(), ErrReentrantTick.Error()) {
		t.Fatalf("reentrant Tick call was not logged, got %q", buf.String())
	}
}
