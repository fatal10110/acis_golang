package sim

import (
	"slices"
	"testing"
)

// TestEffectsRunInAddOrder runs the queued calls in the order they were
// added, and an empty Effects runs nothing.
func TestEffectsRunInAddOrder(t *testing.T) {
	var got []int
	var fx Effects
	fx.Run()
	for i := range 3 {
		fx.Add(func() { got = append(got, i) })
	}
	fx.Run()
	if want := []int{0, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("ran %v, want %v", got, want)
	}
}
