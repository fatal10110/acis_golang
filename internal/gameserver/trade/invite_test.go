package trade

import (
	"testing"
	"time"
)

// TakeInvite consumes only a request of its own kind, and nothing whose
// requester left the world after asking.
func TestBookTakeInvite(t *testing.T) {
	now := time.Unix(10, 0)
	clock := func() time.Time { return now }

	t.Run("requester left", func(t *testing.T) {
		book := NewBook(clock)
		if res := book.Invite(KindParty, 1, 2, true); res.Status != RequestStarted {
			t.Fatalf("Invite = %+v", res)
		}
		book.Leave(1)
		if id, ok := book.TakeInvite(KindParty, 2); ok {
			t.Fatalf("TakeInvite after the requester left = %d, want nothing", id)
		}
		if book.HoldsRequest(2) || book.ProcessingRequest(1) {
			t.Fatal("a request survived being taken")
		}
	})
	t.Run("other kind", func(t *testing.T) {
		book := NewBook(clock)
		book.Invite(KindParty, 1, 2, true)
		if _, ok := book.TakeInvite(KindCommandChannel, 2); ok {
			t.Fatal("a command channel answer took a party invitation")
		}
		if res := book.Answer(2, true); res.Status != AnswerMissing {
			t.Fatalf("a trade answer found a party invitation: %+v", res)
		}
		if !book.HoldsRequest(2) {
			t.Fatal("the party invitation was dropped by an answer of another kind")
		}
		if id, ok := book.TakeInvite(KindParty, 2); !ok || id != 1 {
			t.Fatalf("TakeInvite = %d %v, want requester 1", id, ok)
		}
		if book.HoldsRequest(2) || book.ProcessingRequest(1) {
			t.Fatal("the taken request still holds either side")
		}
	})
}
