package creature

import (
	"sync"
	"testing"
)

func TestHealthShiftCapsAndLeavesALethalChangeToTheCaller(t *testing.T) {
	t.Parallel()
	current := 50.0
	h := NewHealth(&current)

	if written, lethal := h.Shift(80, 100); !written || lethal || current != 100 {
		t.Fatalf("Shift(+80) = %v/%v, current %v; want written, capped at 100", written, lethal, current)
	}
	if written, lethal := h.Shift(-30, 100); !written || lethal || current != 70 {
		t.Fatalf("Shift(-30) = %v/%v, current %v; want written, 70", written, lethal, current)
	}
	if written, lethal := h.Shift(-70, 100); written || !lethal || current != 70 {
		t.Fatalf("Shift(-70) = %v/%v, current %v; want lethal with nothing written", written, lethal, current)
	}

	dead := 0.0
	d := NewHealth(&dead)
	if written, lethal := d.Shift(10, 100); written || lethal || dead != 0 {
		t.Fatalf("Shift on dead health = %v/%v, current %v; want neither, 0", written, lethal, dead)
	}
}

// TestHealthShiftAndDamageSum pins that Shift and a hit taken alongside it
// both land: neither erases the other.
func TestHealthShiftAndDamageSum(t *testing.T) {
	t.Parallel()
	current := 10_000.0
	h := NewHealth(&current)
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(2)
		go func() { defer wg.Done(); h.Shift(-3, 20_000) }()
		go func() { defer wg.Done(); h.Damage(7) }()
	}
	wg.Wait()
	if got := h.Current(); got != 9_000 {
		t.Fatalf("current = %v, want 9000 (100 x (3 + 7) taken)", got)
	}
}
