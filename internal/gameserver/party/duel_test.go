package party

import (
	"slices"
	"testing"
)

// edits returns the leaders of the Edited notices among notices, in order.
func edits(notices []Notice) []*member {
	var out []*member
	for _, n := range notices {
		if e, ok := n.(Edited[*member]); ok {
			out = append(out, e.Leader)
		}
	}
	return out
}

// TestEditedOnEveryMemberChange pins Party's onPartyEdit hooks
// (Party.java:177, 351, 416): a member joining, a member leaving, the
// leader disconnecting and the party breaking up each report one edit,
// naming a leader of the party, so a party duel it fights is cancelled.
// A failed request edits nothing.
func TestEditedOnEveryMemberChange(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(4)
	formParty(t, r, ms[0], ms[1])

	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
		t.Fatalf("BeginInvite = %v", status)
	}
	if got := edits(r.Answer(ms[0], ms[2], true)); !slices.Equal(got, []*member{ms[0]}) {
		t.Fatalf("join edits = %v, want one by A", got)
	}
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
		t.Fatalf("BeginInvite = %v", status)
	}
	if got := edits(r.Answer(ms[0], ms[3], false)); len(got) != 0 {
		t.Fatalf("declined invite edits = %v, want none", got)
	}
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
		t.Fatalf("BeginInvite = %v", status)
	}
	r.Answer(ms[0], ms[3], true)

	if got := edits(r.Leave(ms[3], Left)); !slices.Equal(got, []*member{ms[0]}) {
		t.Fatalf("leave edits = %v, want one by A", got)
	}
	if got := edits(r.Leave(ms[0], Disconnected)); !slices.Equal(got, []*member{ms[1]}) {
		t.Fatalf("disconnecting leader edits = %v, want one by the new leader B", got)
	}
	if got := edits(r.ChangeLeader(ms[1], ms[2].name)); len(got) != 0 {
		t.Fatalf("leader change edits = %v, want none", got)
	}
	if got := edits(r.Leave(ms[1], Left)); !slices.Equal(got, []*member{ms[2]}) {
		t.Fatalf("disband edits = %v, want one by C", got)
	}
	if r.InParty(ms[1].id) || r.InParty(ms[2].id) {
		t.Fatal("the party survived its disbanding")
	}
}

// TestEditedOnExpelAndLeaderDisband: an expelled member and a leader
// leaving, which disbands the party, each edit it once.
func TestEditedOnExpelAndLeaderDisband(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(3)
	formParty(t, r, ms...)
	if got := edits(r.Expel(ms[0].id, ms[2].name)); !slices.Equal(got, []*member{ms[0]}) {
		t.Fatalf("expel edits = %v, want one by A", got)
	}
	if got := edits(r.Expel(ms[1].id, ms[0].name)); len(got) != 0 {
		t.Fatalf("expel by a non-leader edits = %v, want none", got)
	}
	formParty(t, r, ms[0], ms[2])
	if got := edits(r.Leave(ms[0], Left)); !slices.Equal(got, []*member{ms[0]}) {
		t.Fatalf("leader leaving edits = %v, want one by A", got)
	}
}

// TestLeaveChannelSilently pins CommandChannel.removeParty as a party duel
// uses it (CommandChannel.java:189-211): out of three parties, the party's
// channel window closes and the others see it go, with no message; out of
// two, the channel disbands for both. A party in no channel, or a player in
// no party, changes nothing.
func TestLeaveChannelSilently(t *testing.T) {
	r, ms := threeChannelParties(t)
	got := noticeKinds(r.LeaveChannelSilently(ms[3].id))
	if want := []string{"close:CD", "update:ABEF:C"}; !slices.Equal(got, want) {
		t.Fatalf("leaving one of three = %v, want %v", got, want)
	}
	if r.SameChannel(ms[2].id, ms[0].id) || !r.SameChannel(ms[0].id, ms[4].id) {
		t.Fatal("channel membership wrong after the silent leave")
	}
	if got := r.LeaveChannelSilently(ms[2].id); got != nil {
		t.Fatalf("a party in no channel = %v, want nothing", noticeKinds(got))
	}

	got = noticeKinds(r.LeaveChannelSilently(ms[5].id))
	disbanded := "msg" + string(rune('0'+MsgChannelDisbanded))
	if want := []string{"close:AB", disbanded + ":AB:", "close:EF", disbanded + ":EF:"}; !slices.Equal(got, want) {
		t.Fatalf("leaving one of two = %v, want %v", got, want)
	}
	if _, ok := r.Channel(ms[0].id); ok {
		t.Fatal("channel survived with one party")
	}
	if got := r.LeaveChannelSilently(99); got != nil {
		t.Fatalf("a player in no party = %v, want nothing", noticeKinds(got))
	}
}
