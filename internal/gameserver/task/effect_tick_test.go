package task

import (
	"testing"
	"time"
)

// The effect sweep interval bounds how late an effect tick or expiry may
// land (parity exception, #4932). Changing it means revisiting that bound and
// the effect package documentation.
func TestEffectTickIsOneSecond(t *testing.T) {
	if EffectTick != time.Second {
		t.Fatalf("EffectTick = %v, want 1s", EffectTick)
	}
}
