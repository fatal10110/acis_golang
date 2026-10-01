package trade

import (
	"testing"
	"time"
)

// TestRequestUnlessRefusalComesAfterBusy pins RequestUnless: a busy side is
// reported before an outside refusal, as TradeRequest.java checks the busy
// states before the block list, and a refused request records nothing, so
// neither side is busy afterwards.
func TestRequestUnlessRefusalComesAfterBusy(t *testing.T) {
	book := NewBook(time.Now)

	if res := book.RequestUnless(1, 2, true); res.Status != RequestRefused {
		t.Fatalf("refused request status = %v, want refused", res.Status)
	}
	if book.ProcessingTransaction(1) || book.ProcessingTransaction(2) {
		t.Fatal("a refused request left a side busy")
	}

	book.Request(1, 2)
	if res := book.RequestUnless(1, 3, true); res.Status != RequestRequesterBusy {
		t.Fatalf("busy requester status = %v, want requester busy", res.Status)
	}
	if res := book.RequestUnless(3, 2, true); res.Status != RequestTargetBusy {
		t.Fatalf("busy target status = %v, want target busy", res.Status)
	}
	if res := book.RequestUnless(3, 4, false); res.Status != RequestStarted {
		t.Fatalf("unrefused request status = %v, want started", res.Status)
	}
}
