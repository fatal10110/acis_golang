package trade

import (
	"testing"
	"time"
)

// TestBookFriendInvitationSharesTheSlot pins the one pending-request slot
// (Player._activeRequester and _requestExpireTime): a friend invitation
// keeps both sides as busy for a trade request as a pending trade request
// does, and a pending trade request refuses a friend invitation.
func TestBookFriendInvitationSharesTheSlot(t *testing.T) {
	book := NewBook(time.Now)
	if res := book.Invite(KindFriend, 1, 2, false); res.Status != RequestStarted {
		t.Fatalf("friend Invite = %+v, want started", res)
	}
	if res := book.Request(3, 2); res.Status != RequestTargetBusy {
		t.Fatalf("trade request to the invited target = %v, want target busy", res.Status)
	}
	if res := book.Request(1, 3); res.Status != RequestRequesterBusy {
		t.Fatalf("trade request from the inviter = %v, want requester busy", res.Status)
	}
	if !book.HoldsRequest(2) || !book.ProcessingTransaction(1) {
		t.Fatal("the friend invitation holds neither side")
	}

	book.Request(3, 4)
	if res := book.Invite(KindFriend, 5, 4, false); res.Status != RequestTargetBusy {
		t.Fatalf("friend Invite to a target holding a trade request = %+v, want target busy", res)
	}
	if res := book.Invite(KindFriend, 5, 3, false); res.Status != RequestTargetBusy {
		t.Fatalf("friend Invite to a target waiting on its trade request = %+v, want target busy", res)
	}
	// The inviter's own busy state does not refuse a friend invitation.
	if res := book.Invite(KindFriend, 4, 6, false); res.Status != RequestStarted {
		t.Fatalf("friend Invite from a player holding a trade request = %+v, want started", res)
	}
}

// TestBookAnswerKindStaysInItsSlot pins answers to the shared slot: an
// answer of another kind neither consumes nor reaches the request held.
func TestBookAnswerKindStaysInItsSlot(t *testing.T) {
	book := NewBook(time.Now)
	book.Request(1, 2)
	if _, ok := book.TakeRequest(KindFriend, 2); ok {
		t.Fatal("a friend answer took a trade request")
	}
	if res := book.Answer(2, true); res.Status != AnswerAccepted || res.RequesterID != 1 {
		t.Fatalf("trade Answer after a stray friend answer = %+v, want accepted from 1", res)
	}

	book.Invite(KindFriend, 3, 4, false)
	if res := book.Answer(4, true); res.Status != AnswerMissing {
		t.Fatalf("a trade answer found a friend invitation: %+v", res)
	}
	if taken, ok := book.TakeRequest(KindFriend, 4); !ok || taken.RequesterID != 3 || taken.RequesterLeft {
		t.Fatalf("TakeRequest = %+v %v, want requester 3 still there", taken, ok)
	}
}

// TestBookRequesterClockIsShared pins Player.onTransactionRequest and
// onTransactionResponse: every request a login sends restarts one clock
// that all of its pending requests share, and the first answer to any of
// them stops it, ending the others and freeing the requester.
func TestBookRequesterClockIsShared(t *testing.T) {
	now := time.Unix(10, 0)
	book := NewBook(func() time.Time { return now })

	book.Invite(KindFriend, 1, 2, false)
	now = now.Add(10 * time.Second)
	book.Invite(KindFriend, 1, 3, false)
	now = now.Add(10 * time.Second)
	if !book.HoldsRequest(2) {
		t.Fatal("the first invitation expired on its own deadline, not the shared latest one")
	}

	if taken, ok := book.TakeRequest(KindFriend, 3); !ok || taken.RequesterID != 1 {
		t.Fatalf("TakeRequest(3) = %+v %v, want requester 1", taken, ok)
	}
	if _, ok := book.TakeRequest(KindFriend, 2); ok {
		t.Fatal("an invitation outlived the answer to another of its requester's")
	}
	if book.ProcessingRequest(1) {
		t.Fatal("the requester stays busy after an answer")
	}

	now = now.Add(RequestTimeout)
	book.Invite(KindCommandChannel, 1, 2, false)
	book.Invite(KindCommandChannel, 1, 3, false)
	if _, ok := book.TakeInvite(KindCommandChannel, 2); !ok {
		t.Fatal("the first command channel answer found nothing")
	}
	if _, ok := book.TakeInvite(KindCommandChannel, 3); ok {
		t.Fatal("a command channel invitation outlived the answer to another")
	}
}

// TestBookRequesterLeftIsTaken pins a friend invitation whose requester
// left before the answer: it is still taken, marked as from a login that is
// gone, while a later login under the same id is free at once.
func TestBookRequesterLeftIsTaken(t *testing.T) {
	book := NewBook(time.Now)
	book.Invite(KindFriend, 1, 2, false)
	book.Invite(KindFriend, 1, 3, false)
	book.Leave(1)
	if book.ProcessingRequest(1) {
		t.Fatal("a later login inherits the requests the earlier one sent")
	}
	taken, ok := book.TakeRequest(KindFriend, 2)
	if !ok || taken.RequesterID != 1 || !taken.RequesterLeft {
		t.Fatalf("TakeRequest = %+v %v, want requester 1, left", taken, ok)
	}
	if res := book.Invite(KindFriend, 1, 4, false); res.Status != RequestStarted {
		t.Fatalf("later login's Invite = %+v, want started", res)
	}
	if _, ok := book.TakeRequest(KindFriend, 3); ok {
		t.Fatal("the earlier login's other invitation outlived the answer")
	}
	if !book.HoldsRequest(4) {
		t.Fatal("answering the earlier login's invitation ended the later login's")
	}
}

// TestBookLeftRequesterClockOutlivesTradeAndRoomAnswers pins the
// partner-gone branch of the trade and party room answers: answering a
// request whose requester left only clears the answering side, so the
// requester's other pending requests stay answerable until they expire.
func TestBookLeftRequesterClockOutlivesTradeAndRoomAnswers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(book *Book)
	}{
		{"trade accept", func(book *Book) {
			book.Request(1, 2)
			book.Invite(KindFriend, 1, 3, false)
			book.Leave(1)
			if res := book.Answer(2, true); res.Status != AnswerAccepted || !res.RequesterLeft {
				t.Fatalf("trade Answer = %+v, want accepted from a left requester", res)
			}
		}},
		{"trade deny", func(book *Book) {
			book.Request(1, 2)
			book.Invite(KindFriend, 1, 3, false)
			book.Leave(1)
			if res := book.Answer(2, false); res.Status != AnswerDenied || !res.RequesterLeft {
				t.Fatalf("trade Answer = %+v, want denied from a left requester", res)
			}
		}},
		{"party room", func(book *Book) {
			book.Invite(KindPartyRoom, 1, 2, false)
			book.Invite(KindFriend, 1, 3, false)
			book.Leave(1)
			if _, ok := book.TakeInvite(KindPartyRoom, 2); ok {
				t.Fatal("a room answer reached a requester that left")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			book := NewBook(time.Now)
			tc.answer(book)
			if !book.HoldsRequest(3) {
				t.Fatal("answering the left requester's other request ended this one")
			}
			taken, ok := book.TakeRequest(KindFriend, 3)
			if !ok || taken.RequesterID != 1 || !taken.RequesterLeft {
				t.Fatalf("TakeRequest(3) = %+v %v, want requester 1, left", taken, ok)
			}
		})
	}
}

// TestBookLeftRequesterOtherRequestsStillExpire pins that a left
// requester's surviving requests still end on its clock.
func TestBookLeftRequesterOtherRequestsStillExpire(t *testing.T) {
	now := time.Unix(10, 0)
	book := NewBook(func() time.Time { return now })
	book.Request(1, 2)
	book.Invite(KindFriend, 1, 3, false)
	book.Leave(1)
	book.Answer(2, false)
	now = now.Add(RequestTimeout)
	if _, ok := book.TakeRequest(KindFriend, 3); ok {
		t.Fatal("a left requester's invitation outlived its clock")
	}
}
