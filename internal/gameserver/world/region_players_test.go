package world

import (
	"slices"
	"testing"
)

// TestRegion_playersHoldThePlayerSubsetOfObjects pins the players list
// against adds, same-id replacements and both remove paths, and that
// objects keeps its own swap-remove order.
func TestRegion_playersHoldThePlayerSubsetOfObjects(t *testing.T) {
	r := newRegion(0, 0)
	p1 := &regionTestPlayer{regionTestObject{id: 1}}
	n2 := &regionTestObject{id: 2}
	p3 := &regionTestPlayer{regionTestObject{id: 3}}
	n4 := &regionTestObject{id: 4}
	for _, o := range []Tracked{p1, n2, p3, n4} {
		r.add(o)
	}
	assertRegionPlayers(t, r, 1, 3)

	r.remove(1)
	if got := objectIDs(r.appendObjects(nil)); !sameIDs(got, []int32{4, 2, 3}) {
		t.Fatalf("objects after remove = %v, want [4 2 3]", got)
	}
	assertRegionPlayers(t, r, 3)

	// A same-id replacement swaps the player for an NPC and back.
	r.add(&regionTestObject{id: 3})
	assertRegionPlayers(t, r)
	p3again := &regionTestPlayer{regionTestObject{id: 3}}
	r.add(p3again)
	r.add(p3again)
	assertRegionPlayers(t, r, 3)
	if got := objectIDs(r.appendObjects(nil)); !sameIDs(got, []int32{4, 2, 3}) {
		t.Fatalf("objects after replacements = %v, want [4 2 3]", got)
	}

	if r.removeIfSame(3, p3) {
		t.Fatal("removeIfSame dropped a replaced occupant")
	}
	if !r.removeIfSame(3, p3again) {
		t.Fatal("removeIfSame kept the current occupant")
	}
	assertRegionPlayers(t, r)
}

func assertRegionPlayers(t *testing.T, r *Region, want ...int32) {
	t.Helper()
	got := objectIDs(r.appendPlayersExcept(nil, 0))
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("players = %v, want %v", got, want)
	}
	if len(r.players) != len(want) {
		t.Fatalf("len(players) = %d, want %d", len(r.players), len(want))
	}
}

// TestRegion_freshRegionAllocatesOnFirstAdd pins that an unused region holds
// no index map or object storage, and reads and removes on it see nothing.
func TestRegion_freshRegionAllocatesOnFirstAdd(t *testing.T) {
	r := newRegion(5, 6)
	if r.index != nil || r.objects != nil || r.players != nil {
		t.Fatal("fresh region allocated storage before its first add")
	}
	if got := r.appendObjectsExcept(nil, 1); len(got) != 0 {
		t.Fatalf("fresh region objects = %v", objectIDs(got))
	}
	r.remove(1)
	if r.removeIfSame(1, &regionTestObject{id: 1}) {
		t.Fatal("removeIfSame dropped an object from a fresh region")
	}
	r.add(&regionTestObject{id: 1})
	if r.index == nil {
		t.Fatal("add left the index unallocated")
	}
	if got := objectIDs(r.appendObjects(nil)); !sameIDs(got, []int32{1}) {
		t.Fatalf("objects = %v, want [1]", got)
	}
}

// TestState_KnownPlayersMatchesKnownListPlayers pins KnownPlayers to the
// player-kind members of AppendKnown: the 3x3 neighborhood, t excluded,
// NPCs and objects beyond it skipped.
func TestState_KnownPlayersMatchesKnownListPlayers(t *testing.T) {
	s := New()
	x, y := regionCenter(100, 100)
	nx, _ := regionCenter(101, 100)
	fx, _ := regionCenter(103, 100)
	a := &regionTestPlayer{regionTestObject{id: 1}}
	b := &regionTestPlayer{regionTestObject{id: 2}}
	far := &regionTestPlayer{regionTestObject{id: 3}}
	n := &regionTestObject{id: 4}
	n2 := &regionTestObject{id: 5}
	s.Spawn(a, x, y, 0, 0)
	s.Spawn(n, x, y, 0, 0)
	s.Spawn(b, nx, y, 0, 0)
	s.Spawn(n2, nx, y, 0, 0)
	s.Spawn(far, fx, y, 0, 0)

	for _, tc := range []struct {
		name string
		t    Tracked
		want []int32
	}{
		{"npc", n, []int32{1, 2}},
		{"player", a, []int32{2}},
		{"neighbor player", b, []int32{1}},
		{"far player", far, nil},
		{"off grid", &regionTestObject{id: 9}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := objectIDs(slices.Collect(s.KnownPlayers(tc.t)))
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("KnownPlayers = %v, want %v", got, tc.want)
			}
			var fromKnown []int32
			for _, o := range s.AppendKnown(nil, tc.t) {
				if isPlayer(o) {
					fromKnown = append(fromKnown, o.ObjectID())
				}
			}
			slices.Sort(fromKnown)
			if !slices.Equal(got, fromKnown) {
				t.Fatalf("KnownPlayers = %v, known-list players = %v", got, fromKnown)
			}
		})
	}

	seen := 0
	for range s.KnownPlayers(n) {
		seen++
		break
	}
	if seen != 1 {
		t.Fatalf("an early break yielded %d players, want 1", seen)
	}
}
