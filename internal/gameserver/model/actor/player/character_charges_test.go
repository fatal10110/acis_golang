package player

import "testing"

func TestCharacterIncreaseChargesClampsToMax(t *testing.T) {
	c := &Character{ID: 1}

	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 2 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 2, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 4 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 4, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(3, 5); !ok || c.Charges() != 5 {
		t.Fatalf("after +3 clamped: Charges() = %d, ok = %v, want 5, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(1, 5); ok || c.Charges() != 5 {
		t.Fatalf("at max: Charges() = %d, ok = %v, want 5, false", c.Charges(), ok)
	}
}

func TestCharacterDecreaseChargesReportsInsufficientCharges(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(2, 5)

	if ok := c.DecreaseCharges(3); ok || c.Charges() != 2 {
		t.Fatalf("DecreaseCharges(3) over available = ok %v, Charges() %d, want false, 2", ok, c.Charges())
	}
	if ok := c.DecreaseCharges(2); !ok || c.Charges() != 0 {
		t.Fatalf("DecreaseCharges(2) = ok %v, Charges() %d, want true, 0", ok, c.Charges())
	}
}

func TestCharacterClearChargesResetsToZero(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(4, 5)

	c.ClearCharges()

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after ClearCharges = %d, want 0", got)
	}
}

func TestCharacterDieClearsCharges(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(1)
	c.IncreaseCharges(3, 5)

	c.Die(nil)

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after Die = %d, want 0", got)
	}
}
