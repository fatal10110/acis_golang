package party

import (
	"slices"
	"testing"
)

// threeChannelParties forms parties AB, CD and EF and joins all three in
// the channel A leads.
func threeChannelParties(t *testing.T) (*Registry[*member], []*member) {
	t.Helper()
	r := NewRegistry[*member](nil)
	ms := newMembers(6)
	formParty(t, r, ms[0], ms[1])
	formParty(t, r, ms[2], ms[3])
	formParty(t, r, ms[4], ms[5])
	r.JoinChannel(ms[0], ms[2])
	r.JoinChannel(ms[0], ms[4])
	return r, ms
}

// noticeKinds names each notice, in order, with its recipients.
func noticeKinds(notices []Notice) []string {
	var out []string
	ids := func(ms []*member) string {
		var s string
		for _, m := range ms {
			s += m.name
		}
		return s
	}
	for _, n := range notices {
		switch n := n.(type) {
		case Msg[*member]:
			out = append(out, "msg"+string(rune('0'+n.ID))+":"+ids(n.To)+":"+n.Name)
		case ChannelClose[*member]:
			out = append(out, "close:"+ids(n.To))
		case ChannelPartyUpdate[*member]:
			out = append(out, "update:"+ids(n.To)+":"+n.Leader.name)
		default:
			out = append(out, "other")
		}
	}
	return out
}

// TestLeaveChannel pins /channelleave on the registry: only a party leader
// in a channel leaves it. Out of three parties, the leaving party's window
// closes, the rest see it go, the party is told it left, then the parties
// still in the channel are told whose party left. Out of two, the channel
// disbands for both and only the leaving party is told it left: no party is
// left in the channel to hear whose.
func TestLeaveChannel(t *testing.T) {
	r, ms := threeChannelParties(t)
	if got := r.LeaveChannel(ms[3]); got != nil {
		t.Fatalf("LeaveChannel by a member = %v, want nothing", noticeKinds(got))
	}
	got := noticeKinds(r.LeaveChannel(ms[2]))
	want := []string{
		"close:CD",
		"update:ABEF:C",
		"msg" + string(rune('0'+MsgLeftChannel)) + ":CD:",
		"msg" + string(rune('0'+MsgPartyLeftChannel)) + ":ABEF:C",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("leaving one of three = %v, want %v", got, want)
	}
	if r.SameChannel(ms[2].id, ms[0].id) || !r.SameChannel(ms[0].id, ms[4].id) {
		t.Fatal("channel membership wrong after the leave")
	}
	if got := r.LeaveChannel(ms[2]); got != nil {
		t.Fatalf("LeaveChannel out of any channel = %v, want nothing", noticeKinds(got))
	}

	got = noticeKinds(r.LeaveChannel(ms[4]))
	disbanded := "msg" + string(rune('0'+MsgChannelDisbanded))
	want = []string{
		"close:AB", disbanded + ":AB:",
		"close:EF", disbanded + ":EF:",
		"msg" + string(rune('0'+MsgLeftChannel)) + ":EF:",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("leaving one of two = %v, want %v", got, want)
	}
	if _, ok := r.Channel(ms[0].id); ok {
		t.Fatal("channel survived with one party")
	}
}

// TestDisbandChannel pins /channeldelete on the registry: only the
// channel's leader, leading its own party, disbands it, for every party.
func TestDisbandChannel(t *testing.T) {
	r, ms := threeChannelParties(t)
	for _, m := range []*member{ms[1], ms[2]} {
		if got := r.DisbandChannel(m); got != nil {
			t.Fatalf("DisbandChannel by %s = %v, want nothing", m.name, noticeKinds(got))
		}
	}
	got := noticeKinds(r.DisbandChannel(ms[0]))
	disbanded := "msg" + string(rune('0'+MsgChannelDisbanded))
	want := []string{"close:AB", disbanded + ":AB:", "close:CD", disbanded + ":CD:", "close:EF", disbanded + ":EF:"}
	if !slices.Equal(got, want) {
		t.Fatalf("disband = %v, want %v", got, want)
	}
	if r.SameChannel(ms[0].id, ms[2].id) {
		t.Fatal("channel survived its disbanding")
	}
}

// TestChannelView pins the channel overview: its leader, every party's
// leader and size in join order, and the member count over all of them.
func TestChannelView(t *testing.T) {
	r, ms := threeChannelParties(t)
	r.Answer(ms[0], &member{id: 7, name: "G"}, true)
	if _, ok := r.Channel(99); ok {
		t.Fatal("Channel for a player in no party = ok")
	}
	view, ok := r.Channel(ms[5].id)
	if !ok || view.Leader != ms[0] || view.MembersCount != 7 || len(view.Parties) != 3 {
		t.Fatalf("Channel = %+v %v", view, ok)
	}
	for i, want := range []ChannelPartyView[*member]{{ms[0], 3}, {ms[2], 2}, {ms[4], 2}} {
		if view.Parties[i] != want {
			t.Fatalf("party %d = %+v, want %+v", i, view.Parties[i], want)
		}
	}
}
