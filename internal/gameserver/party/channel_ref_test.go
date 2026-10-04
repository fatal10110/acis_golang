package party

import "testing"

// ChannelRef names a command channel by identity: every member of it gets
// an equal ref, another channel an unequal one, and a ref keeps following
// the channel's leader and member count, down to a disbanded channel that
// keeps its last leader and counts no member.
func TestChannelRef(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(9)
	formParty(t, r, ms[0], ms[1])
	formParty(t, r, ms[2], ms[3], ms[4])
	formParty(t, r, ms[5], ms[6])
	formParty(t, r, ms[7], ms[8])

	if _, ok := r.ChannelRef(ms[0].id); ok {
		t.Fatal("ChannelRef of a party outside any channel = ok")
	}
	r.JoinChannel(ms[0], ms[2])
	r.JoinChannel(ms[5], ms[7])

	first, ok := r.ChannelRef(ms[1].id)
	if !ok {
		t.Fatal("ChannelRef of a channel member = !ok")
	}
	if same, _ := r.ChannelRef(ms[4].id); same != first {
		t.Fatal("two members of one channel got unequal refs")
	}
	if other, _ := r.ChannelRef(ms[6].id); other == first {
		t.Fatal("members of two channels got equal refs")
	}
	if got := first.MembersCount(); got != 5 {
		t.Fatalf("MembersCount = %d, want 5", got)
	}
	if got := first.Leader(); got != ms[0] {
		t.Fatalf("Leader = %d, want %d", got.id, ms[0].id)
	}

	r.ChangeLeader(ms[0], ms[1].name)
	if got := first.Leader(); got != ms[1] {
		t.Fatalf("Leader after the hand-over = %d, want %d", got.id, ms[1].id)
	}

	r.Leave(ms[1], Left)
	if _, ok := r.ChannelRef(ms[3].id); ok {
		t.Fatal("ChannelRef after the leading party left = ok")
	}
	if got := first.MembersCount(); got != 0 {
		t.Fatalf("MembersCount of the disbanded channel = %d, want 0", got)
	}
	if got := first.Leader(); got != ms[1] {
		t.Fatalf("Leader of the disbanded channel = %d, want its last %d", got.id, ms[1].id)
	}
}
