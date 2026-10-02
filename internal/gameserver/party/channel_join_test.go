package party

import "testing"

// JoinFormedChannel joins a party into the requester's existing channel as
// JoinChannel does, but forms none when the requester's party is in no
// channel: an answer authorized only to join must not form one for free.
func TestJoinFormedChannelNeverForms(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(6)
	formParty(t, r, ms[0], ms[1])
	formParty(t, r, ms[2], ms[3])
	formParty(t, r, ms[4], ms[5])

	if got := r.JoinFormedChannel(ms[0], ms[2]); got != nil {
		t.Fatalf("JoinFormedChannel without a channel = %v, want nothing", messages(got))
	}
	if r.SameChannel(ms[0].id, ms[2].id) {
		t.Fatal("JoinFormedChannel formed a channel")
	}

	r.JoinChannel(ms[0], ms[2])
	joined := r.JoinFormedChannel(ms[0], ms[4])
	if !hasMessage(joined, MsgJoinedChannel, ms[5].id) || !r.SameChannel(ms[1].id, ms[5].id) {
		t.Fatalf("JoinFormedChannel into a channel = %v, want the party joined", messages(joined))
	}
}
