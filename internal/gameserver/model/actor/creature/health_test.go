package creature

import "testing"

func TestHealthDamageClampsAndReportsFirstDeath(t *testing.T) {
	current := 10.0
	h := NewHealth(&current)

	if h.Damage(-5) {
		t.Fatal("Damage(-5) = true, want false")
	}
	if current != 10 {
		t.Fatalf("current after negative damage = %v, want 10", current)
	}

	if h.Damage(4) {
		t.Fatal("Damage(4) = true, want false")
	}
	if current != 6 {
		t.Fatalf("current after partial damage = %v, want 6", current)
	}

	if !h.Damage(99) {
		t.Fatal("Damage(99) = false, want first death")
	}
	if current != 0 {
		t.Fatalf("current after lethal damage = %v, want 0", current)
	}

	if h.Damage(1) {
		t.Fatal("Damage after death = true, want false")
	}
}

func TestHealthSetCurrentLeavesDeadHealthAlone(t *testing.T) {
	current := 0.0
	h := NewHealth(&current)

	h.SetCurrent(5)

	if current != 0 {
		t.Fatalf("current after SetCurrent on dead health = %v, want 0", current)
	}
}
