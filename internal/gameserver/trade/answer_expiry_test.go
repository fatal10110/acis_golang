package trade

import (
	"testing"
	"time"
)

// TestBookAnswerAfterExpiryFindsNothing pins Player.getActiveRequester
// dropping an expired requester: an answer that comes at or after the
// request's timeout, accepted or refused, finds no request (AnswerMissing),
// opens no session, and leaves neither side busy.
func TestBookAnswerAfterExpiryFindsNothing(t *testing.T) {
	for _, tc := range []struct {
		name          string
		accept        bool
		requesterLeft bool
	}{
		{"accept", true, false},
		{"refuse", false, false},
		{"accept after requester left", true, true},
		{"refuse after requester left", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(10, 0)
			book := NewBook(func() time.Time { return now })
			book.Request(1, 2)
			if tc.requesterLeft {
				book.Leave(1)
			}
			now = now.Add(RequestTimeout)

			res := book.Answer(2, tc.accept)
			if res.Status != AnswerMissing || res.TargetID != 2 || res.RequesterID != 0 {
				t.Fatalf("Answer = %+v, want missing for target 2 with no requester", res)
			}
			for _, id := range []int32{1, 2} {
				if _, ok := book.Session(id); ok {
					t.Fatalf("Session(%d) opened by an expired request", id)
				}
				if book.ProcessingTransaction(id) {
					t.Fatalf("ProcessingTransaction(%d) = true after the request expired", id)
				}
			}
			if again := book.Answer(2, tc.accept); again.Status != AnswerMissing {
				t.Fatalf("second Answer = %+v, want missing", again)
			}
		})
	}
}

// TestBookAnswerJustBeforeExpiry pins the boundary: the last instant before
// the timeout still answers the live request.
func TestBookAnswerJustBeforeExpiry(t *testing.T) {
	for _, tc := range []struct {
		accept bool
		want   AnswerStatus
	}{
		{true, AnswerAccepted},
		{false, AnswerDenied},
	} {
		now := time.Unix(10, 0)
		book := NewBook(func() time.Time { return now })
		book.Request(1, 2)
		now = now.Add(RequestTimeout - time.Nanosecond)
		if res := book.Answer(2, tc.accept); res.Status != tc.want || res.RequesterID != 1 {
			t.Fatalf("Answer(accept=%v) = %+v, want %v from requester 1", tc.accept, res, tc.want)
		}
	}
}
