package party

import "testing"

func TestRewardGroup(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(5)
	ms[3].level = 52
	formParty(t, r, ms[0], ms[1])
	formParty(t, r, ms[2], ms[3])

	if _, ok := r.RewardGroup(ms[4].id); ok {
		t.Fatal("a player in no party has a reward group")
	}
	g, ok := r.RewardGroup(ms[1].id)
	if !ok || g.InChannel || g.ChannelLevel != 0 || !sameMembers(g.Members, ms[0], ms[1]) {
		t.Fatalf("party group = %+v %v, want its two members outside a channel", g, ok)
	}
	g.Members[0] = ms[4]
	if g, _ := r.RewardGroup(ms[1].id); !sameMembers(g.Members, ms[0], ms[1]) {
		t.Fatal("the reward group aliases the party's member list")
	}

	// In a channel, every party's members, party by party, at the channel's
	// level: its highest party's, here set by a member of the other party.
	r.JoinChannel(ms[0], ms[2])
	for _, m := range ms[:4] {
		g, ok := r.RewardGroup(m.id)
		if !ok || !g.InChannel || g.ChannelLevel != 52 || !sameMembers(g.Members, ms[:4]...) {
			t.Fatalf("channel group for %s = %+v %v, want all four at level 52", m.name, g, ok)
		}
	}
}

func sameMembers(got []*member, want ...*member) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
