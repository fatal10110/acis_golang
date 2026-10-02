package player

import "testing"

// TestIsLooterOrInLooterParty pins who may take loot reserved to an owner:
// the owner, a member of the owner's party, and a member of another party
// in the owner's command channel. A player in another party outside that
// channel, or in no party, may not.
func TestIsLooterOrInLooterParty(t *testing.T) {
	// Players 1 and 2 share party 1; player 3 leads party 2, which shares
	// channel 1 with party 1; player 4 is in party 3, in no channel; player
	// 5 is in no party.
	g := policyGraph{
		party:   map[int32]int{1: 1, 2: 1, 3: 2, 4: 3},
		channel: map[int]int{1: 1, 2: 1},
	}
	picker := &Character{ID: 1, social: g}
	for _, tc := range []struct {
		what  string
		owner int32
		want  bool
	}{
		{"own loot", 1, true},
		{"party member's loot", 2, true},
		{"command channel member's loot", 3, true},
		{"another party's loot", 4, false},
		{"a partyless player's loot", 5, false},
	} {
		if got := picker.IsLooterOrInLooterParty(tc.owner); got != tc.want {
			t.Errorf("%s: IsLooterOrInLooterParty(%d) = %v, want %v", tc.what, tc.owner, got, tc.want)
		}
	}

	solo := &Character{ID: 5, social: g}
	if solo.IsLooterOrInLooterParty(1) || !solo.IsLooterOrInLooterParty(5) {
		t.Error("a partyless player may take only its own loot")
	}
}
