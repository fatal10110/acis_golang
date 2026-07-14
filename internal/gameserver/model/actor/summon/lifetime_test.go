package summon

import "testing"

func TestInitialNextConsumeTime(t *testing.T) {
	tests := []struct {
		name                         string
		totalLifeTime, steps, itemID int
		want                         int
	}{
		{"no consume item", 10000, 1, 0, -1},
		{"zero steps", 10000, 0, 57, -1},
		{"even split", 10000, 1, 57, 5000},
		{"three steps", 1200000, 3, 57, 900000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InitialNextConsumeTime(tt.totalLifeTime, tt.steps, tt.itemID); got != tt.want {
				t.Errorf("InitialNextConsumeTime() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestTick_Sequence walks a small servitor through its full life, checking
// each tick's expired/dueForUpkeep flags and the running countdown state.
// totalLifeTime=10000 with 1 consume step means the first (only) upkeep
// checkpoint sits at 5000; each 2000-cost tick should cross a checkpoint on
// the ticks where the remaining time steps from above the checkpoint to at
// or below it.
func TestTick_Sequence(t *testing.T) {
	state := LifetimeState{
		TimeRemaining:       10000,
		TotalLifeTime:       10000,
		NextItemConsumeTime: InitialNextConsumeTime(10000, 1, 57),
		ItemConsumeSteps:    1,
	}
	if state.NextItemConsumeTime != 5000 {
		t.Fatalf("setup: NextItemConsumeTime = %d, want 5000", state.NextItemConsumeTime)
	}

	type step struct {
		wantRemaining int
		wantExpired   bool
		wantUpkeep    bool
	}
	steps := []step{
		{8000, false, false},
		{6000, false, false},
		{4000, false, true}, // crosses the 5000 checkpoint
		{2000, false, false},
		{0, false, true}, // crosses the second checkpoint at 0
		{-2000, true, false},
	}

	for i, want := range steps {
		next, expired, upkeep := Tick(state, 2000)
		if expired != want.wantExpired {
			t.Errorf("tick %d: expired = %v, want %v", i+1, expired, want.wantExpired)
		}
		if upkeep != want.wantUpkeep {
			t.Errorf("tick %d: dueForUpkeep = %v, want %v", i+1, upkeep, want.wantUpkeep)
		}
		if next.TimeRemaining != want.wantRemaining {
			t.Errorf("tick %d: TimeRemaining = %d, want %d", i+1, next.TimeRemaining, want.wantRemaining)
		}
		state = next
		if expired {
			break
		}
	}
}

func TestTick_NeverConsumes(t *testing.T) {
	state := LifetimeState{
		TimeRemaining:       10000,
		TotalLifeTime:       10000,
		NextItemConsumeTime: -1, // no consume item
		ItemConsumeSteps:    0,
	}
	for i := 0; i < 5; i++ {
		next, expired, upkeep := Tick(state, 1000)
		if upkeep {
			t.Fatalf("tick %d: dueForUpkeep = true, want false (no consume item)", i+1)
		}
		if expired {
			t.Fatalf("tick %d: expired unexpectedly at TimeRemaining=%d", i+1, next.TimeRemaining)
		}
		state = next
	}
}
