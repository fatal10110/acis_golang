package network

import (
	"slices"
	"testing"
)

// noOwner is a world lookup that finds nobody.
func noOwner(int32) (*livePlayer, bool) { return nil, false }

func TestSelectingOwnersSettlesOfflineWithNoSelection(t *testing.T) {
	var s selectingOwners
	var ran []string
	owner, ok := s.settle(7, noOwner, func() { ran = append(ran, "offline") }, func() { ran = append(ran, "retry") })
	if ok || owner != nil {
		t.Fatalf("settle returned a session %v with nobody in the world", owner)
	}
	if !slices.Equal(ran, []string{"offline"}) {
		t.Fatalf("ran %v, want the offline settle at once", ran)
	}
}

func TestSelectingOwnersLeavesAnOwnerInTheWorldToTheCaller(t *testing.T) {
	var s selectingOwners
	live := &livePlayer{}
	var ran []string
	owner, ok := s.settle(7, func(id int32) (*livePlayer, bool) { return live, id == 7 },
		func() { ran = append(ran, "offline") }, func() { ran = append(ran, "retry") })
	if !ok || owner != live {
		t.Fatalf("settle returned %v, %v; want the owner in the world", owner, ok)
	}
	if len(ran) != 0 {
		t.Fatalf("ran %v, want nothing run for an owner in the world", ran)
	}
}

// TestSelectingOwnersHoldsSettlesUntilTheLastSelectionEnds: settles of an
// owner a selection is under way for wait, in order, until the last
// selection of that owner ends; another owner's settle does not.
func TestSelectingOwnersHoldsSettlesUntilTheLastSelectionEnds(t *testing.T) {
	var s selectingOwners
	var ran []string
	record := func(name string) func() { return func() { ran = append(ran, name) } }
	s.begin(7)
	s.begin(7)
	if _, ok := s.settle(7, noOwner, record("offline items"), record("retry items")); ok {
		t.Fatal("settle returned a session while a selection is under way")
	}
	if _, ok := s.settle(7, nil, record("offline collar"), record("retry collar")); ok {
		t.Fatal("settle returned a session while a selection is under way")
	}
	s.settle(8, noOwner, record("offline other"), record("retry other"))
	if !slices.Equal(ran, []string{"offline other"}) {
		t.Fatalf("ran %v during the selection, want only the other owner's settle", ran)
	}
	s.end(7)
	if !slices.Equal(ran, []string{"offline other"}) {
		t.Fatalf("ran %v with a selection still under way, want nothing more", ran)
	}
	s.end(7)
	if want := []string{"offline other", "retry items", "retry collar"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v after the last selection ended, want %v", ran, want)
	}
	s.settle(7, noOwner, record("offline after"), record("retry after"))
	if got := ran[len(ran)-1]; got != "offline after" {
		t.Fatalf("settle after the selection ended ran %q, want it offline", got)
	}
	s.end(7) // an end with no selection under way does nothing
}
