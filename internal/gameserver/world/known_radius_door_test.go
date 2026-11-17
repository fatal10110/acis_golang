package world

import (
	"slices"
	"testing"
)

// A door's body radius (16) widens a radius scan from a bodied creature: a
// door up to radius + selfRadius + 16 units away is known, one unit past that
// is not, and a point object gets no such widening.
func TestForEachKnownInRadiusWidensForDoorCandidate(t *testing.T) {
	const (
		// doorRadius is the body radius every door has (model/door cannot be
		// imported here without a cycle).
		doorRadius = 16
		radius     = 500
		selfRadius = 20
	)
	s := New()
	self := &bodiedTestObject{regionTestObject: regionTestObject{id: 1}, radius: selfRadius}
	s.Spawn(self, 0, 0, 0, 0)

	reach := radius + selfRadius
	for _, o := range []struct {
		obj  Tracked
		dist int
	}{
		{&bodiedTestObject{regionTestObject{id: 2}, doorRadius}, reach},      // door at the creature widening alone
		{&bodiedTestObject{regionTestObject{id: 3}, doorRadius}, reach + 1},  // door 1 past radius + selfRadius
		{&bodiedTestObject{regionTestObject{id: 4}, doorRadius}, reach + 16}, // door 16 past: last widened unit
		{&bodiedTestObject{regionTestObject{id: 5}, doorRadius}, reach + 17}, // door 17 past: out
		{&regionTestObject{id: 6}, reach + 1},                                // point object 1 past: out
		{&regionTestObject{id: 7}, reach},                                    // point object at the creature widening: in
	} {
		s.Spawn(o.obj, o.dist, 0, 0, 0)
	}

	if got, want := knownIDs(s.ForEachKnownInRadius, self, radius), []int32{2, 3, 4, 7}; !slices.Equal(got, want) {
		t.Fatalf("ForEachKnownInRadius(%d) = %v, want %v", radius, got, want)
	}
}
