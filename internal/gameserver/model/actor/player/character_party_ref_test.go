package player

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// refGraph is a policyGraph whose parties, by id, have a leader and
// members; partyOf maps a player to its party's id.
type refGraph struct {
	policyGraph
	partyOf map[int32]party.ID
	leader  map[party.ID]*Character
	members map[party.ID][]*Character
}

func (g refGraph) PartyOf(id int32) (party.ID, bool) {
	p, ok := g.partyOf[id]
	return p, ok
}

func (g refGraph) PartyByID(id party.ID) (*Character, []*Character, bool) {
	m, ok := g.members[id]
	return g.leader[id], m, ok
}

// A party reference names its party by id: it reads the party's current
// leader and members, matches another reference to the same party only,
// and reads empty once the party is gone. A character in no party has
// none.
func TestPartyRefFollowsItsParty(t *testing.T) {
	a, b, c := &Character{ID: 1}, &Character{ID: 2}, &Character{ID: 3}
	g := refGraph{
		partyOf: map[int32]party.ID{1: 7, 2: 7},
		leader:  map[party.ID]*Character{7: b},
		members: map[party.ID][]*Character{7: {a, b}},
	}
	for _, ch := range []*Character{a, b, c} {
		ch.social = g
	}
	ra, ok := a.Party()
	if !ok {
		t.Fatal("a has no party")
	}
	rb, _ := b.Party()
	if !ra.Same(rb) {
		t.Fatal("two members' references differ")
	}
	if _, ok := c.Party(); ok {
		t.Fatal("c, in no party, has one")
	}
	if ra.Leader() != b || !slices.Equal(ra.Members(), []*Character{a, b}) {
		t.Fatalf("leader %v members %v, want b and [a b]", ra.Leader(), ra.Members())
	}
	if ra.Same(PartyRef{social: g, id: 8}) {
		t.Fatal("references to two parties match")
	}

	g.members[7] = []*Character{a, b, c}
	g.leader[7] = a
	if ra.Leader() != a || len(ra.Members()) != 3 {
		t.Fatalf("after the party changed: leader %v members %v", ra.Leader(), ra.Members())
	}

	delete(g.members, 7)
	if ra.Leader() != nil || ra.Members() != nil {
		t.Fatalf("a gone party reads leader %v members %v", ra.Leader(), ra.Members())
	}
	if _, ok := (&Character{ID: 9}).Party(); ok {
		t.Fatal("a character without a social graph has a party")
	}
}
