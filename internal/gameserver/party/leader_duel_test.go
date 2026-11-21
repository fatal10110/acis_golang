package party

import (
	"slices"
	"testing"
)

// TestDisconnectingLeaderHandsOverToDuelingMember pins a documented parity
// exception (#5000; porting-playbook section 3). The reference promotes
// the first remaining member through Party.changePartyLeader
// (Party.java:405-410), which returns early for a member in a duel
// (Party.java:466), so the party keeps a leader who has already left it.
// Go always hands the party to the first remaining member; the edit that
// follows still names a party leader, so the party duel is cancelled as in
// the reference.
func TestDisconnectingLeaderHandsOverToDuelingMember(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(3)
	formParty(t, r, ms...)
	ms[1].duel = true

	notices := r.Leave(ms[0], Disconnected)

	if !hasMessage(notices, MsgBecameLeader, ms[1].id) || !hasMessage(notices, MsgBecameLeader, ms[2].id) {
		t.Fatalf("handover messages = %v, want B named leader to both who stay", messages(notices))
	}
	if hasMessage(notices, MsgBecameLeader, ms[0].id) {
		t.Fatal("the disconnected leader was told of the handover")
	}
	if got := edits(notices); !slices.Equal(got, []*member{ms[1]}) {
		t.Fatalf("edits = %v, want one by the new leader B (cancels the party duel)", got)
	}
	v, ok := r.View(ms[2].id)
	if !ok || v.Leader != ms[1] || !slices.Equal(v.Members, []*member{ms[1], ms[2]}) {
		t.Fatalf("party after the handover = %+v %v, want B leading B and C", v, ok)
	}
	if r.InParty(ms[0].id) {
		t.Fatal("the disconnected leader is still in the party")
	}
}
