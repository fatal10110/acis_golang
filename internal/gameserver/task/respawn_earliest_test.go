package task

import (
	"slices"
	"testing"
	"time"
)

// Of two respawns armed for one slot, the one that comes due first fires,
// whichever order they were armed in; nothing fires again after it.
func TestRespawnAddEarliestKeepsTheEarlierDeadline(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second time.Duration
	}{
		{"shorter then longer", 60 * time.Second, 600 * time.Second},
		{"longer then shorter", 600 * time.Second, 60 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.UnixMilli(0)
			now := start
			effects := &respawnFakeEffects{}
			r, err := NewRespawn(effects, func() time.Time { return now })
			if err != nil {
				t.Fatalf("NewRespawn() error = %v", err)
			}
			r.AddEarliest("slot", start.Add(tc.first))
			r.AddEarliest("slot", start.Add(tc.second))

			now = start.Add(59 * time.Second)
			r.Tick()
			if got := effects.take(); len(got) != 0 {
				t.Fatalf("Tick at 59s = %v, want none", got)
			}
			now = start.Add(60 * time.Second)
			r.Tick()
			if got, want := effects.take(), []string{"slot"}; !slices.Equal(got, want) {
				t.Fatalf("Tick at 60s = %v, want %v", got, want)
			}
			now = start.Add(600 * time.Second)
			r.Tick()
			if got := effects.take(); len(got) != 0 {
				t.Fatalf("Tick at 600s = %v, want none", got)
			}
		})
	}
}
