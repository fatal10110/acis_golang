package fishing

import "testing"

// ungradedLures are the equippable lures the grade table does not list,
// sold only by GM shops (#5118).
var ungradedLures = []int32{6528, 7067, 7068, 7069, 7070, 8496, 8497, 8498, 8499, 8500, 8501, 8502, 8503, 8504}

// TestCastRefusalUngradedLure pins #5118: a lure with no grade is refused
// after the no-lure check, while every graded lure may be cast.
func TestCastRefusalUngradedLure(t *testing.T) {
	ready := fisher{rod: true}
	for _, id := range ungradedLures {
		if got := CastRefusal(ready, id); got != RefuseUngradedLure {
			t.Errorf("CastRefusal with lure %d = %d, want RefuseUngradedLure", id, got)
		}
		if got := CheckDelay(id, 5000); got != 0 {
			t.Errorf("CheckDelay(%d) = %v, want 0 for an ungraded lure", id, got)
		}
	}
	if got := CastRefusal(ready, 0); got != RefuseNoLure {
		t.Errorf("CastRefusal with no lure = %d, want RefuseNoLure", got)
	}
	if got := CastRefusal(fisher{rod: true, water: true}, 6528); got != RefuseInWater {
		t.Errorf("CastRefusal in water with an ungraded lure = %d, want RefuseInWater", got)
	}
	// One source of truth: a lure is cast exactly when it has a look period.
	for id := int32(6000); id <= 9000; id++ {
		graded := CastRefusal(ready, id) == RefuseNone
		if looks := CheckDelay(id, 5000) > 0; graded != looks {
			t.Errorf("lure %d: castable %t, has a look period %t", id, graded, looks)
		}
	}
}
