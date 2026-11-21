package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestWaterHandoverKeepsSwimming pins a crossing between two touching water
// zones (#4954): whichever order the old zone's exit and the new zone's
// entry are reported in, the last swim change leaves the creature swimming;
// leaving the last water zone stops it.
func TestWaterHandoverKeepsSwimming(t *testing.T) {
	ix := NewIndex()
	west := NewWater(1, testCuboid(t, 0, 1000, 0, 1000))
	east := NewWater(2, testCuboid(t, 1001, 2000, 0, 1000))
	ix.Add(west)
	ix.Add(east)

	s := &swimmingStub{memberStub: memberStub{id: 1, class: ClassSummon, at: location.Location{X: 500, Y: 500}}}
	s.Enter(ix, s)
	settle := func(x int) {
		t.Helper()
		s.swims = nil
		s.at = location.Location{X: x, Y: 500}
		s.Settle(ix, s)
	}
	for _, step := range []struct {
		name string
		x    int
		want bool
	}{
		{"west to east", 1500, true},
		{"east to west", 500, true},
		{"west to dry land", -500, false},
		{"dry land to east", 1500, true},
		{"east to dry land", 2500, false},
	} {
		settle(step.x)
		if len(s.swims) == 0 {
			t.Fatalf("%s: no swim change", step.name)
		}
		if got := s.swims[len(s.swims)-1]; got != step.want {
			t.Fatalf("%s: swim changes %v, last %v, want %v", step.name, s.swims, got, step.want)
		}
		if got := s.ZoneFlags().Has(FlagWater); got != step.want {
			t.Fatalf("%s: in water %v, want %v", step.name, got, step.want)
		}
	}
	if west.Inside(s) || east.Inside(s) {
		t.Fatal("the creature on dry land is still inside a water zone")
	}
}
