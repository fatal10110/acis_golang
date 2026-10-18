package spawn

import (
	"testing"
	"time"
)

// The jitter is a whole number of seconds drawn from the 2*random+1 values
// in [-random, random], so every respawn delay is a whole number of seconds.
func TestCalculateRespawnDelayDrawsWholeSeconds(t *testing.T) {
	entry := Entry{RespawnDelay: 30 * time.Second, RespawnRandom: 2 * time.Second}
	seen := map[time.Duration]bool{}
	for range 2000 {
		got := CalculateRespawnDelay(entry)
		if got%time.Second != 0 {
			t.Fatalf("CalculateRespawnDelay() = %v, want a whole number of seconds", got)
		}
		seen[got] = true
	}
	for s := 28; s <= 32; s++ {
		if !seen[time.Duration(s)*time.Second] {
			t.Fatalf("CalculateRespawnDelay() never drew %ds over 2000 draws; drew %v", s, seen)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("CalculateRespawnDelay() drew %d distinct values, want the 5 of [28s, 32s]", len(seen))
	}
}
