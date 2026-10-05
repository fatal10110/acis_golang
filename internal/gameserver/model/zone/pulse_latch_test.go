package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestPulseLatchHandsOverToARacingEntry pins the pulse latch of the effect
// and damage zones: the first entry starts the pulse, later ones start
// nothing while it runs, and the stopping task's PulseStopped reports, and
// keeps the latch for, an occupant that entered after its last look. Once
// the zone is empty, PulseStopped frees the latch and the next entry starts
// a fresh pulse. A damage zone gone dormant frees it too.
func TestPulseLatchHandsOverToARacingEntry(t *testing.T) {
	form := testCuboid(t, 0, 1000, 0, 1000)
	effect, err := NewEffect(1, form, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	castle := commons.NewStatSet()
	castle.Set("castleId", "1")
	damage, err := NewDamage(2, form, castle)
	if err != nil {
		t.Fatal(err)
	}
	damage.Armed = true
	siege := true
	damage.SiegeActive = func() bool { return siege }

	for _, tc := range []struct {
		name    string
		k       Kind
		start   *func()
		stopped func() bool
	}{
		{"effect", effect, &effect.StartPulse, effect.PulseStopped},
		{"damage", damage, &damage.StartPulse, damage.PulseStopped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			starts := 0
			*tc.start = func() { starts++ }
			a := &memberStub{id: 1, class: ClassPlayer, at: location.Location{X: 10, Y: 10}}
			b := &memberStub{id: 2, class: ClassPlayer, at: location.Location{X: 20, Y: 20}}
			Revalidate(tc.k, a)
			Revalidate(tc.k, b)
			if starts != 1 {
				t.Fatalf("starts after two entries = %d, want 1", starts)
			}
			if !tc.stopped() {
				t.Fatal("PulseStopped with occupants inside = false, want true")
			}
			Remove(tc.k, a)
			Remove(tc.k, b)
			Revalidate(tc.k, a)
			if starts != 1 {
				t.Fatalf("entry while the handed-over latch is held started %d pulses, want none", starts-1)
			}
			Remove(tc.k, a)
			if tc.stopped() {
				t.Fatal("PulseStopped on an empty zone = true, want false")
			}
			Revalidate(tc.k, a)
			if starts != 2 {
				t.Fatalf("entry after the latch was freed: starts = %d, want 2", starts)
			}
			Remove(tc.k, a)
			tc.stopped()
		})
	}

	// A dormant damage zone frees the latch even with occupants inside.
	starts := 0
	damage.StartPulse = func() { starts++ }
	a := &memberStub{id: 1, class: ClassPlayer, at: location.Location{X: 10, Y: 10}}
	Revalidate(damage, a)
	siege = false
	if damage.PulseStopped() {
		t.Fatal("PulseStopped on a dormant trap = true, want false")
	}
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
	}
}
