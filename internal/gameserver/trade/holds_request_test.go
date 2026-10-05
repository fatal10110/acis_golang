package trade

import (
	"testing"
	"time"
)

// TestBookHoldsRequestIsActiveRequester pins HoldsRequest to
// Player.getActiveRequester() != null (Player.java:3028-3034): only the
// target of an unanswered request is held, not its requester; the hold ends
// with the requester's clock (an answer or the timeout), except that a
// target with a trade window open stays held past the timeout until the
// window closes.
func TestBookHoldsRequestIsActiveRequester(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	book := NewBook(func() time.Time { return now })

	book.Request(1, 2)
	if !book.HoldsRequest(2) {
		t.Fatal("target of a pending trade request is not held")
	}
	if book.HoldsRequest(1) {
		t.Fatal("requester of a pending trade request is held")
	}
	now = now.Add(RequestTimeout)
	if book.HoldsRequest(2) {
		t.Fatal("target still held after the request expired")
	}

	// An answered request no longer holds its target, even inside the
	// window it opened.
	book.Request(1, 2)
	if res := book.Answer(2, true); res.Status != AnswerAccepted {
		t.Fatalf("Answer = %+v, want accepted", res)
	}
	if book.HoldsRequest(2) || book.HoldsRequest(1) {
		t.Fatal("a trade opened from an answered request holds a participant")
	}

	// An invitation received with the window open outlives its expiry
	// while the window stays open, then goes with it.
	if res := book.Invite(KindParty, 3, 2, true); res.Status != RequestStarted {
		t.Fatalf("party Invite = %+v, want started", res)
	}
	now = now.Add(RequestTimeout + time.Second)
	if !book.HoldsRequest(2) {
		t.Fatal("expired invitation stopped holding a target with a trade window open")
	}
	if res := book.Cancel(2); res.Status != CancelDone {
		t.Fatalf("Cancel = %+v, want done", res)
	}
	if book.HoldsRequest(2) {
		t.Fatal("expired invitation still holds its target after the window closed")
	}
}
