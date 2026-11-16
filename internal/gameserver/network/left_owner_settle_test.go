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
	owner, ok := s.settle(7, noOwner, func() { ran = append(ran, "offline") }, func() { ran = append(ran, "retry") }, false)
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
		func() { ran = append(ran, "offline") }, func() { ran = append(ran, "retry") }, false)
	if !ok || owner != live {
		t.Fatalf("settle returned %v, %v; want the owner in the world", owner, ok)
	}
	if len(ran) != 0 {
		t.Fatalf("ran %v, want nothing run for an owner in the world", ran)
	}
}

// TestSelectingOwnersHoldsSettlesUntilTheSelectionEnds: settles of an owner
// a selection is under way for wait, in order, until that selection ends;
// another owner's settle does not. A second selection of the owner is
// refused while the first is under way.
func TestSelectingOwnersHoldsSettlesUntilTheSelectionEnds(t *testing.T) {
	var s selectingOwners
	var ran []string
	record := func(name string) func() { return func() { ran = append(ran, name) } }
	if !s.begin(7) {
		t.Fatal("first selection refused")
	}
	if s.begin(7) {
		t.Fatal("second selection of the owner claimed while the first is under way")
	}
	if !s.begin(8) {
		t.Fatal("another owner's selection refused")
	}
	s.end(8)
	if _, ok := s.settle(7, noOwner, record("offline items"), record("retry items"), false); ok {
		t.Fatal("settle returned a session while a selection is under way")
	}
	if _, ok := s.settle(7, nil, record("offline collar"), record("retry collar"), false); ok {
		t.Fatal("settle returned a session while a selection is under way")
	}
	s.settle(8, noOwner, record("offline other"), record("retry other"), false)
	if !slices.Equal(ran, []string{"offline other"}) {
		t.Fatalf("ran %v during the selection, want only the other owner's settle", ran)
	}
	s.end(7)
	if want := []string{"offline other", "retry items", "retry collar"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v after the selection ended, want %v", ran, want)
	}
	if !s.begin(7) {
		t.Fatal("selection refused after the earlier one ended")
	}
	s.end(7)
	s.settle(7, noOwner, record("offline after"), record("retry after"), false)
	if got := ran[len(ran)-1]; got != "offline after" {
		t.Fatalf("settle after the selection ended ran %q, want it offline", got)
	}
	s.end(7) // an end with no selection under way does nothing
}

// gatedSettle models settleWithLeftOwner on s for an owner out of the world:
// a settle that runs offline as name, held as a retry that comes back
// draining.
func gatedSettle(s *selectingOwners, ran *[]string, name string, draining bool) {
	retry := func() { gatedSettle(s, ran, name, true) }
	s.settle(7, noOwner, func() { *ran = append(*ran, name) }, retry, draining)
}

// TestSelectingOwnersHoldsASettleArrivingWhileTheHeldOnesRun: a settle that
// arrives after the last selection ended but while its held settles are still
// running waits behind them, so a corpse's collar is not deleted ahead of its
// items.
func TestSelectingOwnersHoldsASettleArrivingWhileTheHeldOnesRun(t *testing.T) {
	var s selectingOwners
	var ran []string
	s.begin(7)
	// The items settle is held; as end runs it again, the collar settle
	// arrives from the decay goroutine before the items have gone anywhere.
	s.settle(7, noOwner, func() { ran = append(ran, "items") }, func() {
		gatedSettle(&s, &ran, "collar", false)
		gatedSettle(&s, &ran, "items", true)
	}, false)
	s.end(7)
	if want := []string{"items", "collar"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v, want the held items ahead of the collar that arrived while they ran (%v)", ran, want)
	}
	gatedSettle(&s, &ran, "after", false)
	if got := ran[len(ran)-1]; got != "after" {
		t.Fatalf("settle after the held ones ran %q, want it offline at once", got)
	}
}

// TestSelectingOwnersHandsTheRestToASelectionBeginningWhileTheyRun: a
// selection of the owner beginning while the held settles of an earlier one
// run holds the rest of them, in order, until it ends.
func TestSelectingOwnersHandsTheRestToASelectionBeginningWhileTheyRun(t *testing.T) {
	var s selectingOwners
	var ran []string
	s.begin(7)
	s.settle(7, noOwner, func() { ran = append(ran, "first") }, func() {
		gatedSettle(&s, &ran, "first", true)
		s.begin(7)
	}, false)
	gatedSettle(&s, &ran, "second", false)
	gatedSettle(&s, &ran, "third", false)
	s.end(7)
	if want := []string{"first"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v, want only the settle run before the new selection began", ran)
	}
	gatedSettle(&s, &ran, "fourth", false)
	s.end(7)
	if want := []string{"first", "second", "third", "fourth"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v after the new selection ended, want %v", ran, want)
	}
}

// TestSelectingOwnersReholdsARunningSettleAtTheFront: a held settle that a
// new selection catches as it runs again is held at the front, ahead of those
// held behind it.
func TestSelectingOwnersReholdsARunningSettleAtTheFront(t *testing.T) {
	var s selectingOwners
	var ran []string
	s.begin(7)
	s.settle(7, noOwner, func() { ran = append(ran, "first") }, func() {
		s.begin(7)
		gatedSettle(&s, &ran, "first", true)
	}, false)
	gatedSettle(&s, &ran, "second", false)
	s.end(7)
	if len(ran) != 0 {
		t.Fatalf("ran %v with a new selection under way, want nothing", ran)
	}
	s.end(7)
	if want := []string{"first", "second"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v, want %v", ran, want)
	}
}
