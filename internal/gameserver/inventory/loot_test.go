package inventory

import "testing"

// soloPicker is a player in no party: it may take only its own loot.
type soloPicker int32

func (p soloPicker) IsLooterOrInLooterParty(ownerID int32) bool { return ownerID == int32(p) }

// partyPicker may also take loot reserved to the members of its party.
type partyPicker struct {
	id      int32
	members []int32
}

func (p partyPicker) IsLooterOrInLooterParty(ownerID int32) bool {
	if ownerID == p.id {
		return true
	}
	for _, m := range p.members {
		if m == ownerID {
			return true
		}
	}
	return false
}

func TestLootLockedAdmitsOwnerAndParty(t *testing.T) {
	for _, tc := range []struct {
		name   string
		owner  int32
		picker LootPicker
		want   bool
	}{
		{"unowned", 0, soloPicker(1), false},
		{"owner", 1, soloPicker(1), false},
		{"stranger", 2, soloPicker(1), true},
		{"party member", 2, partyPicker{id: 1, members: []int32{2}}, false},
		{"outside the party", 3, partyPicker{id: 1, members: []int32{2}}, true},
	} {
		if got := LootLocked(tc.owner, tc.picker); got != tc.want {
			t.Errorf("%s: LootLocked = %v, want %v", tc.name, got, tc.want)
		}
	}
}
