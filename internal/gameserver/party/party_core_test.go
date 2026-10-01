package party

import (
	"testing"
	"time"
)

// The command channel can only be formed by a level 5 clan leader, and no
// clan exists yet, so its membership rules are pinned here on the registry
// directly rather than through packets.

type member struct {
	id       int32
	name     string
	level    int
	departed bool
}

func (m *member) ObjectID() int32       { return m.id }
func (m *member) Level() int            { return m.level }
func (m *member) CharacterName() string { return m.name }
func (m *member) Departed() bool        { return m.departed }

func newMembers(n int) []*member {
	out := make([]*member, n)
	for i := range out {
		out[i] = &member{id: int32(i + 1), name: string(rune('A' + i)), level: 10 + i}
	}
	return out
}

// formParty forms a party of ms led by ms[0].
func formParty(t *testing.T, r *Registry[*member], ms ...*member) {
	t.Helper()
	for _, m := range ms[1:] {
		if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
			t.Fatalf("BeginInvite = %v", status)
		}
		r.Answer(ms[0], m, true)
	}
}

func messages(notices []Notice) []MessageID {
	var out []MessageID
	for _, n := range notices {
		if m, ok := n.(Msg[*member]); ok {
			out = append(out, m.ID)
		}
	}
	return out
}

func hasMessage(notices []Notice, id MessageID, to int32) bool {
	for _, n := range notices {
		if m, ok := n.(Msg[*member]); ok && m.ID == id {
			for _, r := range m.To {
				if r.id == to {
					return true
				}
			}
		}
	}
	return false
}

func TestChannelFormsJoinsAndDisbands(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(6)
	formParty(t, r, ms[0], ms[1])
	formParty(t, r, ms[2], ms[3])
	formParty(t, r, ms[4], ms[5])

	if status, leader := r.ChannelInvite(ms[0].id, ms[3].id); status != ChannelInviteReady || leader != ms[2] {
		t.Fatalf("ChannelInvite = %v %v, want ready to the target party's leader", status, leader)
	}
	if forms, ok := r.ChannelAnswerForms(ms[0].id, ms[2].id); !forms || !ok {
		t.Fatalf("ChannelAnswerForms = %v %v, want a new channel", forms, ok)
	}
	formed := r.JoinChannel(ms[0], ms[2])
	if !hasMessage(formed, MsgChannelFormed, ms[1].id) || !hasMessage(formed, MsgJoinedChannel, ms[3].id) {
		t.Fatalf("formation messages = %v", messages(formed))
	}
	if !r.SameChannel(ms[1].id, ms[3].id) || r.SameChannel(ms[1].id, ms[5].id) {
		t.Fatal("channel membership wrong after formation")
	}
	if status, _ := r.ChannelInvite(ms[1].id, ms[4].id); status != ChannelInviteNotLeader {
		t.Fatalf("member invitation = %v, want not leader", status)
	}
	if status, _ := r.ChannelInvite(ms[2].id, ms[4].id); status != ChannelInviteNotLeader {
		t.Fatalf("invitation by a party leader not leading the channel = %v, want not leader", status)
	}
	if status, _ := r.ChannelInvite(ms[4].id, ms[2].id); status != ChannelInviteTargetInChannel {
		t.Fatalf("invitation of a channel party = %v, want target in channel", status)
	}

	joined := r.JoinChannel(ms[0], ms[4])
	var update ChannelPartyUpdate[*member]
	for _, n := range joined {
		if u, ok := n.(ChannelPartyUpdate[*member]); ok {
			update = u
		}
	}
	if !update.Added || update.Leader != ms[4] || update.Count != 2 || len(update.To) != 4 {
		t.Fatalf("join update = %+v, want the new party added for the four already in", update)
	}

	// Ousting one of three parties keeps the channel.
	status, ousted := r.Oust(ms[0], ms[5].id)
	if status != OustDone || !hasMessage(ousted, MsgDismissedFromChannel, ms[5].id) || !hasMessage(ousted, MsgPartyDismissedFromChannel, ms[1].id) {
		t.Fatalf("oust = %v %v", status, messages(ousted))
	}
	if r.SameChannel(ms[0].id, ms[4].id) || !r.SameChannel(ms[0].id, ms[2].id) {
		t.Fatal("channel membership wrong after the oust")
	}
	if status, _ := r.Oust(ms[0], ms[5].id); status != OustInvalidTarget {
		t.Fatalf("second oust = %v, want invalid target", status)
	}
	if status, _ := r.Oust(ms[2], ms[0].id); status != OustNotAuthorized {
		t.Fatalf("oust by a non-leader = %v, want not authorized", status)
	}

	// The channel leader's party dispersing disbands the channel.
	dispersed := r.Leave(ms[0], Left)
	if !hasMessage(dispersed, MsgChannelDisbanded, ms[3].id) || !hasMessage(dispersed, MsgPartyDispersed, ms[1].id) {
		t.Fatalf("dispersal messages = %v", messages(dispersed))
	}
	if r.SameChannel(ms[2].id, ms[3].id) {
		t.Fatal("channel survived its leading party")
	}
	if v, ok := r.View(ms[2].id); !ok || v.InChannel {
		t.Fatalf("remaining party view = %+v %v, want a party out of any channel", v, ok)
	}
}

func TestChannelLeaderFollowsPartyLeader(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(5)
	formParty(t, r, ms[0], ms[1], ms[2])
	formParty(t, r, ms[3], ms[4])
	r.JoinChannel(ms[0], ms[3])

	// A disconnecting channel leader hands both its party and the channel
	// to the next member.
	notices := r.Leave(ms[0], Disconnected)
	if !hasMessage(notices, MsgChannelLeaderNow, ms[4].id) || !hasMessage(notices, MsgBecameLeader, ms[2].id) {
		t.Fatalf("handover messages = %v", messages(notices))
	}
	if status, _ := r.Oust(ms[1], ms[4].id); status != OustDone {
		t.Fatalf("oust by the new channel leader = %v, want done", status)
	}
}

func TestPartyLevelFollowsMembers(t *testing.T) {
	r := NewRegistry[*member](nil)
	ms := newMembers(3)
	formParty(t, r, ms[0], ms[1], ms[2])
	if v, _ := r.View(ms[0].id); v.Level != 12 {
		t.Fatalf("level = %d, want the highest member's 12", v.Level)
	}
	ms[1].level = 40
	r.RecalculateLevel(ms[1].id)
	if v, _ := r.View(ms[0].id); v.Level != 40 {
		t.Fatalf("level after a level-up = %d, want 40", v.Level)
	}
	r.Leave(ms[1], Left)
	if v, _ := r.View(ms[0].id); v.Level != 12 {
		t.Fatalf("level after the top member left = %d, want 12", v.Level)
	}
}

func TestInvitationWaitsAndExpires(t *testing.T) {
	now := time.Unix(100, 0)
	r := NewRegistry[*member](func() time.Time { return now })
	ms := newMembers(4)
	formParty(t, r, ms[0], ms[1])

	if status, loot := r.BeginInvite(ms[0].id, 4); status != InviteReady || loot != LootFindersKeepers {
		t.Fatalf("BeginInvite = %v %v, want ready with the party's rule", status, loot)
	}
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteWaiting {
		t.Fatalf("second BeginInvite = %v, want waiting", status)
	}
	now = now.Add(InviteTimeout)
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
		t.Fatalf("BeginInvite after the timeout = %v, want ready", status)
	}
	if status, _ := r.BeginInvite(ms[1].id, 0); status != InviteNotLeader {
		t.Fatalf("member BeginInvite = %v, want not leader", status)
	}
	if status, _ := r.BeginInvite(ms[2].id, 5); status != InviteBadLoot {
		t.Fatalf("BeginInvite with rule 5 = %v, want bad loot", status)
	}
	// A player already in a party joins no other.
	r.Answer(ms[2], ms[1], true)
	if r.InParty(ms[2].id) || !r.SameParty(ms[0].id, ms[1].id) {
		t.Fatal("a party member was taken into a second party")
	}
}
