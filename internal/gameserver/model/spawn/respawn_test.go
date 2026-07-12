package spawn

import (
	"testing"
	"time"
)

func TestCalculateRespawnDelayNoRespawnWhenDelayIsZero(t *testing.T) {
	entry := Entry{RespawnDelay: 0, RespawnRandom: 5 * time.Second}
	if got := CalculateRespawnDelay(entry); got != 0 {
		t.Fatalf("CalculateRespawnDelay() = %v, want 0", got)
	}
}

func TestCalculateRespawnDelayNoRandomReturnsDelay(t *testing.T) {
	entry := Entry{RespawnDelay: 30 * time.Second, RespawnRandom: 0}
	if got := CalculateRespawnDelay(entry); got != 30*time.Second {
		t.Fatalf("CalculateRespawnDelay() = %v, want 30s", got)
	}
}

func TestCalculateRespawnDelayStaysWithinBounds(t *testing.T) {
	entry := Entry{RespawnDelay: 30 * time.Second, RespawnRandom: 10 * time.Second}
	min, max := 20*time.Second, 40*time.Second

	for i := 0; i < 500; i++ {
		got := CalculateRespawnDelay(entry)
		if got < min || got > max {
			t.Fatalf("CalculateRespawnDelay() = %v, want within [%v, %v]", got, min, max)
		}
	}
}

func TestCalculateRespawnDelayClampsRandomToDelay(t *testing.T) {
	// RespawnRandom larger than RespawnDelay must clamp so the result never
	// goes negative, matching the reference implementation's guarantee.
	entry := Entry{RespawnDelay: 5 * time.Second, RespawnRandom: 50 * time.Second}

	for i := 0; i < 500; i++ {
		got := CalculateRespawnDelay(entry)
		if got < 0 || got > 10*time.Second {
			t.Fatalf("CalculateRespawnDelay() = %v, want within [0, 10s]", got)
		}
	}
}
