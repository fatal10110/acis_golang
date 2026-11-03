package doorhit

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

type shape struct{}

func (shape) GeoX() int               { return 0 }
func (shape) GeoY() int               { return 0 }
func (shape) GeoZ() int               { return 0 }
func (shape) Height() int             { return 0 }
func (shape) GeoData() [][]block.NSWE { return [][]block.NSWE{{block.AllDirections}} }

type strife sevensigns.Cabal

func (s strife) SealOwners() [3]sevensigns.Cabal {
	return [3]sevensigns.Cabal{sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Cabal(s)}
}

// TestDefencesFollowSealOfStrife pins DoorStatus.getPDef/getMDef on the
// Gludio outer gate's template defences (doors.xml 19210001: pDef 644,
// mDef 518): Dawn raises them by a fifth, Dusk cuts them to 30%, and the
// product is truncated.
func TestDefencesFollowSealOfStrife(t *testing.T) {
	obj, err := door.NewObject(1, &door.Template{ID: 19210001, PDef: 644, MDef: 518}, shape{})
	if err != nil {
		t.Fatal(err)
	}
	target := Of(obj)
	for _, tc := range []struct {
		name       string
		strife     door.Strife
		pDef, mDef float64
	}{
		{name: "no seven signs state", pDef: 644, mDef: 518},
		{name: "unowned", strife: strife(sevensigns.NoCabal), pDef: 644, mDef: 518},
		{name: "dawn", strife: strife(sevensigns.Dawn), pDef: 772, mDef: 621},
		{name: "dusk", strife: strife(sevensigns.Dusk), pDef: 193, mDef: 155},
	} {
		obj.SetStrife(tc.strife)
		if got := target.PDef(); got != tc.pDef {
			t.Errorf("%s: P.Def = %v, want %v", tc.name, got, tc.pDef)
		}
		if got := target.MDef(); got != tc.mDef {
			t.Errorf("%s: M.Def = %v, want %v", tc.name, got, tc.mDef)
		}
	}
}
