package fishing

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// fisher is a caster with the given rod, boat, store and water state.
type fisher struct {
	rod, boat, operating, water bool
}

func (f fisher) FishingRod() (item.CrystalType, bool) { return item.CrystalD, f.rod }
func (f fisher) InBoat() bool                         { return f.boat }
func (f fisher) Operating() bool                      { return f.operating }
func (f fisher) InWater() bool                        { return f.water }

// TestCastRefusalOnBoat pins where the boat refusal sits in the reference
// Fishing.useSkill order: after the rod check, ahead of the store and water
// checks.
func TestCastRefusalOnBoat(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    fisher
		want Refusal
	}{
		{"no rod, aboard", fisher{boat: true}, RefuseNoRod},
		{"aboard", fisher{rod: true, boat: true}, RefuseOnBoat},
		{"aboard, running a store, in water", fisher{rod: true, boat: true, operating: true, water: true}, RefuseOnBoat},
		{"ashore, running a store", fisher{rod: true, operating: true}, RefuseOperating},
	} {
		if got := CastRefusal(tc.f, true); got != tc.want {
			t.Errorf("%s: CastRefusal = %d, want %d", tc.name, got, tc.want)
		}
	}
}
