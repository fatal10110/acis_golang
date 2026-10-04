package duel

import "testing"

// TestPartyDuel: PartyDuel tells a party duel's players from a one-on-one
// duel's, still answers once an ended duel waits for its players to leave,
// and reports false for a player in no duel.
func TestPartyDuel(t *testing.T) {
	t.Parallel()
	m, a, b := solo(t)
	if m.PartyDuel(a) || m.PartyDuel(b) {
		t.Fatal("a one-on-one duel reported as a party duel")
	}
	if m.PartyDuel(newFake(9, 0)) {
		t.Fatal("a player in no duel reported in a party duel")
	}

	pm, pa, pa2, pb, pb2 := partyDuel(t)
	for _, p := range fakes(pa, pa2, pb, pb2) {
		if !pm.PartyDuel(p) {
			t.Fatalf("player %d of a party duel not reported in one", p.id)
		}
	}
	if _, ok := pm.PartyEdit(pa2); !ok {
		t.Fatal("PartyEdit did not end the party duel")
	}
	if !pm.PartyDuel(pb) {
		t.Fatal("an ended party duel its players have not left stopped answering")
	}
}
