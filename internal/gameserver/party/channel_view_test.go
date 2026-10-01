package party

import (
	"slices"
	"testing"
)

// Channel lists the channel leader and every member of every party in the
// channel, party by party in join order; a player in no party, or in a
// party outside any channel, has none.
func TestChannelView(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(6)
	formParty(t, r, ms[0], ms[1])
	formParty(t, r, ms[2], ms[3])
	formParty(t, r, ms[4], ms[5])

	if _, ok := r.Channel(ms[0].id); ok {
		t.Fatal("Channel before any channel formed = ok")
	}
	r.JoinChannel(ms[2], ms[0])
	r.JoinChannel(ms[2], ms[4])

	for _, m := range ms {
		view, ok := r.Channel(m.id)
		if !ok || view.Leader != ms[2] {
			t.Fatalf("Channel(%d) = %v %v, want led by %d", m.id, view.Leader, ok, ms[2].id)
		}
		want := []*member{ms[2], ms[3], ms[0], ms[1], ms[4], ms[5]}
		if !slices.Equal(view.Members, want) {
			t.Fatalf("Channel(%d) members = %v, want %v", m.id, view.Members, want)
		}
	}
	if _, ok := r.Channel(99); ok {
		t.Fatal("Channel of a player in no party = ok")
	}
}
