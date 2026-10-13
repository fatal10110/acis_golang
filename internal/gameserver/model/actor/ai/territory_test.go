package ai

import (
	"testing"
	"time"
)

func TestNextOnGrid(t *testing.T) {
	base := time.Unix(1_000, 0)
	for _, tc := range []struct {
		name string
		now  time.Duration
		want time.Duration
	}{
		{"before the deadline", -3 * time.Second, 0},
		{"at the deadline", 0, 10 * time.Second},
		{"between two", 25 * time.Second, 30 * time.Second},
		{"on a later point", 30 * time.Second, 40 * time.Second},
	} {
		next, d := NextOnGrid(base, base.Add(tc.now), 10*time.Second)
		if want := base.Add(tc.want); !next.Equal(want) || d != tc.want-tc.now {
			t.Fatalf("%s: NextOnGrid = %v, %v; want %v, %v", tc.name, next, d, want, tc.want-tc.now)
		}
	}
}

// TestKeepTerritoryResumesTheSweepOnItsGrid pins a new life taking up the
// out-of-territory state its earlier life left: outside, the sweep is armed
// on its grid and an arrival outside opens no point; inside, nothing is
// armed and an arrival outside opens it.
func TestKeepTerritoryResumesTheSweepOnItsGrid(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	memory := &TerritoryMemory{}
	memory.Store(true, time.Now().Add(-25*time.Second))

	brain.KeepTerritory(memory)
	pending := owner.pendingTimers()
	if len(pending) != 1 || pending[0].delay <= 4*time.Second || pending[0].delay > 5*time.Second {
		t.Fatalf("armed sweeps = %d (first delay %v), want one about 5 s on, on the grid", len(pending), pending)
	}
	brain.Arrived()
	assertHookPoints(t, owner, "on an arrival outside")

	inside := NewAttackable(actor(2), &recordingMove{}, &recordingAttack{})
	fresh := inside.actor.(*fakeActor)
	fresh.inTerritory = false
	inside.KeepTerritory(&TerritoryMemory{})
	if n := len(fresh.pendingTimers()); n != 0 {
		t.Fatalf("armed sweeps of a life left inside = %d, want 0", n)
	}
	inside.Arrived()
	assertHookPoints(t, fresh, "on the first arrival outside", HookOutOfTerritory)
}
