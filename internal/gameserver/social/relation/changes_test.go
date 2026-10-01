package relation

import (
	"slices"
	"testing"
)

// TestChangesListsOnlyPairsChangedSinceLoad pins what the shutdown save
// writes: a loaded pair nobody touched, or one set to flags it already had,
// is left out; a new, changed or cleared pair is listed with its current
// flags, lower id first and in pair order. A saved pair stays listed.
func TestChangesListsOnlyPairsChangedSinceLoad(t *testing.T) {
	m := NewManager([]Row{
		{CharID: 1, FriendID: 2, Relation: flagFriends},
		{CharID: 3, FriendID: 4, Relation: flagFriends},
		{CharID: 5, FriendID: 6, Relation: flagLowBlocksHigh},
		{CharID: 7, FriendID: 8, Relation: flagFriends},
	})
	if got := m.Changes(); len(got) != 0 {
		t.Fatalf("Changes after load = %+v, want none", got)
	}

	m.AddFriend(2, 1) // already friends
	m.Block(5, 6)     // already blocked
	if m.RemoveFriend(5, 6) || m.Unblock(6, 5) {
		t.Fatal("clearing a flag the pair does not have reported a change")
	}
	if got := m.Changes(); len(got) != 0 {
		t.Fatalf("Changes after no-op updates = %+v, want none", got)
	}

	m.Block(4, 3)        // a loaded pair gains a flag
	m.RemoveFriend(8, 7) // a loaded pair loses its last flag
	m.AddFriend(20, 10)  // a new pair
	m.Block(12, 11)
	m.Unblock(12, 11) // a new pair cleared again
	want := []Row{
		{CharID: 3, FriendID: 4, Relation: flagFriends | flagHighBlocksLow},
		{CharID: 7, FriendID: 8, Relation: 0},
		{CharID: 10, FriendID: 20, Relation: flagFriends},
		{CharID: 11, FriendID: 12, Relation: 0},
	}
	if got := m.Changes(); !slices.Equal(got, want) {
		t.Fatalf("Changes = %+v, want %+v", got, want)
	}
	if got := m.Changes(); !slices.Equal(got, want) {
		t.Fatalf("Changes read twice = %+v, want %+v", got, want)
	}
}
