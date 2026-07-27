package player

import "testing"

func TestCharacterSetDeathPenaltyLevelClampsToBounds(t *testing.T) {
	c := &Character{ID: 1}

	c.SetDeathPenaltyLevel(-3)
	if got := c.DeathPenaltyLevel(); got != 0 {
		t.Fatalf("SetDeathPenaltyLevel(-3): DeathPenaltyLevel() = %d, want 0", got)
	}

	c.SetDeathPenaltyLevel(999)
	if got := c.DeathPenaltyLevel(); got != maxDeathPenaltyLevel {
		t.Fatalf("SetDeathPenaltyLevel(999): DeathPenaltyLevel() = %d, want %d", got, maxDeathPenaltyLevel)
	}

	c.SetDeathPenaltyLevel(7)
	if got := c.DeathPenaltyLevel(); got != 7 {
		t.Fatalf("SetDeathPenaltyLevel(7): DeathPenaltyLevel() = %d, want 7", got)
	}
}

func TestCharacterReduceDeathPenaltyLevelDecrementsToFloorZero(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(2)

	if got := c.ReduceDeathPenaltyLevel(); got != 1 {
		t.Fatalf("first ReduceDeathPenaltyLevel() = %d, want 1", got)
	}
	if got := c.ReduceDeathPenaltyLevel(); got != 0 {
		t.Fatalf("second ReduceDeathPenaltyLevel() = %d, want 0", got)
	}
	if got := c.ReduceDeathPenaltyLevel(); got != 0 {
		t.Fatalf("ReduceDeathPenaltyLevel() at zero = %d, want unchanged 0", got)
	}
}
