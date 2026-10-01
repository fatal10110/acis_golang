package party

import (
	"testing"
	"time"
)

// An answer to a requester that has begun leaving the world forms no
// party and joins no one: its own departure already took it out of every
// party, or will once Answer releases the lock.
func TestAnswerRefusesDepartedRequester(t *testing.T) {
	t.Run("partyless requester", func(t *testing.T) {
		r := NewRegistry[*member](func() time.Time { return time.Unix(100, 0) })
		ms := newMembers(2)
		r.BeginInvite(ms[0].id, 0)
		ms[0].departed = true
		if out := r.Answer(ms[0], ms[1], true); len(out) != 0 {
			t.Fatalf("Answer to a departed requester = %v, want nothing", out)
		}
		if r.InParty(ms[0].id) || r.InParty(ms[1].id) {
			t.Fatal("a party formed around a departed requester")
		}
	})
	t.Run("departed target", func(t *testing.T) {
		r := NewRegistry[*member](func() time.Time { return time.Unix(100, 0) })
		ms := newMembers(2)
		r.BeginInvite(ms[0].id, 0)
		ms[1].departed = true
		if out := r.Answer(ms[0], ms[1], true); len(out) != 0 || r.InParty(ms[1].id) {
			t.Fatalf("Answer by a departed target = %v, want nothing", out)
		}
	})
	t.Run("leader of a party", func(t *testing.T) {
		now := time.Unix(100, 0)
		r := NewRegistry[*member](func() time.Time { return now })
		ms := newMembers(3)
		formParty(t, r, ms[0], ms[1])
		r.BeginInvite(ms[0].id, 0)
		ms[0].departed = true
		if out := r.Answer(ms[0], ms[2], true); len(out) != 0 || r.InParty(ms[2].id) {
			t.Fatalf("Answer to a departed leader = %v, want nothing", out)
		}
		// The party stops waiting all the same.
		ms[0].departed = false
		if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
			t.Fatalf("BeginInvite after the answer = %v, want ready", status)
		}
	})
}

// An invitation that reached no one stops its party waiting, so the leader
// may invite again within the invitation timeout.
func TestCancelInviteStopsWaiting(t *testing.T) {
	r := NewRegistry[*member](func() time.Time { return time.Unix(100, 0) })
	ms := newMembers(3)
	formParty(t, r, ms[0], ms[1])
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
		t.Fatalf("BeginInvite = %v", status)
	}
	// Only the leader's own invitation is withdrawn.
	r.CancelInvite(ms[1].id)
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteWaiting {
		t.Fatalf("BeginInvite after a member's CancelInvite = %v, want waiting", status)
	}
	r.CancelInvite(ms[0].id)
	if status, _ := r.BeginInvite(ms[0].id, 0); status != InviteReady {
		t.Fatalf("BeginInvite after CancelInvite = %v, want ready", status)
	}
	r.CancelInvite(ms[2].id) // partyless: nothing to withdraw
}
