package party

import (
	"slices"
	"testing"
)

// PartyRef names a party by identity: every member of it gets an equal
// ref, another party an unequal one, and a ref keeps following the party's
// leader and members, down to a disbanded party that keeps its last leader
// and has no member. A party formed again by the same players is another
// party.
func TestPartyRef(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(6)
	formParty(t, r, ms[0], ms[1], ms[2])
	formParty(t, r, ms[3], ms[4])

	if _, ok := r.PartyRef(ms[5].id); ok {
		t.Fatal("PartyRef of a player in no party = ok")
	}
	first, ok := r.PartyRef(ms[1].id)
	if !ok {
		t.Fatal("PartyRef of a party member = !ok")
	}
	if same, _ := r.PartyRef(ms[2].id); same != first {
		t.Fatal("two members of one party got unequal refs")
	}
	if other, _ := r.PartyRef(ms[3].id); other == first {
		t.Fatal("members of two parties got equal refs")
	}
	if got := first.Members(); !slices.Equal(got, ms[:3]) || first.Leader() != ms[0] {
		t.Fatalf("Members = %v Leader = %d, want the first three led by %d", got, first.Leader().id, ms[0].id)
	}

	r.ChangeLeader(ms[0], ms[2].name)
	if got := first.Leader(); got != ms[2] {
		t.Fatalf("Leader after the hand-over = %d, want %d", got.id, ms[2].id)
	}
	if got := first.Members(); !slices.Equal(got, ms[:3]) {
		t.Fatal("a leader change reordered the members")
	}

	r.Leave(ms[1], Left)
	r.Leave(ms[0], Left)
	if _, ok := r.PartyRef(ms[2].id); ok {
		t.Fatal("PartyRef after the party disbanded = ok")
	}
	if got := first.Members(); len(got) != 0 {
		t.Fatalf("Members of the disbanded party = %v, want none", got)
	}
	if got := first.Leader(); got != ms[2] {
		t.Fatalf("Leader of the disbanded party = %d, want its last, %d", got.id, ms[2].id)
	}

	formParty(t, r, ms[2], ms[0])
	if again, _ := r.PartyRef(ms[2].id); again == first {
		t.Fatal("a party formed again matched the disbanded one")
	}
}
