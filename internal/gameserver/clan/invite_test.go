package clan

import (
	"testing"
	"time"
)

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time { return c.at }

// TestInviteLapses keeps both sides busy until InviteTimeout has passed,
// then answers the invited player as having nothing pending and frees both
// for a new invitation.
func TestInviteLapses(t *testing.T) {
	clock := &fakeClock{at: time.UnixMilli(1_800_000_000_000)}
	b := NewInvites(clock.now)
	if got := b.Send(InviteJoinPledge, 1, "Lead", 2, 0, SubunitMain); got != InviteSent {
		t.Fatalf("send = %v, want InviteSent", got)
	}
	if got := b.Send(InviteJoinPledge, 3, "Other", 2, 0, SubunitMain); got != InviteTargetBusy {
		t.Fatalf("invite to a busy target = %v, want InviteTargetBusy", got)
	}
	if got := b.Send(InviteJoinPledge, 1, "Lead", 3, 0, SubunitMain); got != InviteRequesterBusy {
		t.Fatalf("second invite from a busy requester = %v, want InviteRequesterBusy", got)
	}

	clock.at = clock.at.Add(InviteTimeout - time.Millisecond)
	if e, ok := b.Partner(2); !ok || e.RequesterID != 1 || e.RequesterName != "Lead" {
		t.Fatalf("partner just before the timeout = %+v, %v; want the leader's invitation", e, ok)
	}
	if _, ok := b.Requested(1); !ok {
		t.Fatal("requester's invitation lapsed before the timeout")
	}

	clock.at = clock.at.Add(time.Millisecond)
	if e, ok := b.Partner(2); ok {
		t.Fatalf("partner at the timeout = %+v, want nothing pending", e)
	}
	if e, ok := b.Requested(1); ok {
		t.Fatalf("requested at the timeout = %+v, want nothing pending", e)
	}
	if got := b.Send(InviteJoinPledge, 3, "Other", 2, 0, SubunitMain); got != InviteSent {
		t.Fatalf("invite after the lapse = %v, want InviteSent", got)
	}
	if got := b.Send(InviteJoinPledge, 1, "Lead", 4, 0, SubunitMain); got != InviteSent {
		t.Fatalf("lapsed requester's new invite = %v, want InviteSent", got)
	}
}

// TestInviteLeftFreesRelog lets a requester whose login is gone be invited
// again under the same id, while the player it invited may still answer
// its invitation.
func TestInviteLeftFreesRelog(t *testing.T) {
	clock := &fakeClock{at: time.UnixMilli(1_800_000_000_000)}
	b := NewInvites(clock.now)
	b.Send(InviteJoinPledge, 1, "Lead", 2, 0, SubunitMain)
	b.Left(1)

	if e, ok := b.Partner(2); !ok || e.RequesterID != 1 {
		t.Fatalf("partner of a requester that left = %+v, %v; want its invitation", e, ok)
	}
	if e, ok := b.Requested(1); !ok || e.PartnerID != 2 {
		t.Fatalf("requested by a requester that left = %+v, %v; want its invitation", e, ok)
	}
	if got := b.Send(InviteJoinPledge, 3, "Other", 1, 0, SubunitMain); got != InviteSent {
		t.Fatalf("invite to the relogged id = %v, want InviteSent", got)
	}

	b.Answered(2)
	if _, ok := b.Partner(2); ok {
		t.Fatal("answered invitation still pending")
	}
}

// TestInviteSendPurgesLapsedEntries drops every lapsed entry when a new
// invitation is sent, so pairs that are never looked up again do not stay
// in the book.
func TestInviteSendPurgesLapsedEntries(t *testing.T) {
	clock := &fakeClock{at: time.UnixMilli(1_800_000_000_000)}
	b := NewInvites(clock.now)
	for id := int32(10); id < 20; id += 2 {
		if got := b.Send(InviteJoinPledge, id, "Lead", id+1, 0, SubunitMain); got != InviteSent {
			t.Fatalf("send from %d = %v, want InviteSent", id, got)
		}
	}
	clock.at = clock.at.Add(InviteTimeout)
	if got := b.Send(InviteJoinPledge, 1, "Lead", 2, 0, SubunitMain); got != InviteSent {
		t.Fatalf("send after the lapse = %v, want InviteSent", got)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) != 2 {
		t.Fatalf("entries after send = %d, want only the new pair's 2", len(b.entries))
	}
}
