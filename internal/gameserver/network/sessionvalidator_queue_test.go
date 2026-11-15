package network

import "testing"

func queuedRequest(t *testing.T, v *SessionValidator, account string, linkDone <-chan struct{}) *authRequest {
	t.Helper()
	req := &authRequest{linkDone: linkDone, result: make(chan bool, 1)}
	if !v.enqueue(account, req) {
		t.Fatalf("enqueue for %q refused", account)
	}
	return req
}

func expectAnswer(t *testing.T, req *authRequest, want bool, what string) {
	t.Helper()
	select {
	case got := <-req.result:
		if got != want {
			t.Fatalf("%s: answer = %v, want %v", what, got, want)
		}
	default:
		t.Fatalf("%s: no answer delivered", what)
	}
}

func expectNoAnswer(t *testing.T, req *authRequest, what string) {
	t.Helper()
	select {
	case got := <-req.result:
		t.Fatalf("%s: answer %v delivered, want none", what, got)
	default:
	}
}

// TestSessionValidatorDropsAnswerToEvictedRequest evicts a request and
// queues a replacement for the account: the login server's approval of the
// evicted request runs nothing and never reaches the replacement, which
// gets the next answer.
func TestSessionValidatorDropsAnswerToEvictedRequest(t *testing.T) {
	v := newSessionValidator(0)
	evicted := queuedRequest(t, v, "acc", nil)
	v.Evict("acc")
	expectAnswer(t, evicted, false, "evicted request")

	replacement := queuedRequest(t, v, "acc", nil)
	approvals := 0
	v.ResolveThen("acc", true, func() { approvals++ })
	expectNoAnswer(t, replacement, "replacement after the evicted request's answer")
	if approvals != 0 {
		t.Fatalf("approvals = %d for the evicted request's answer, want 0", approvals)
	}

	v.ResolveThen("acc", false, func() { approvals++ })
	expectAnswer(t, replacement, false, "replacement")
}

// TestSessionValidatorDropsAnswerToAbandonedRequest covers a client that
// stopped waiting (timed out or canceled) before its answer came: the late
// answer is dropped, and the account's next request gets its own answer
// and runs the approval hook.
func TestSessionValidatorDropsAnswerToAbandonedRequest(t *testing.T) {
	v := newSessionValidator(0)
	v.abandon(queuedRequest(t, v, "acc", nil))
	next := queuedRequest(t, v, "acc", nil)

	v.Resolve("acc", true)
	expectNoAnswer(t, next, "next request after the late answer")

	approvals := 0
	v.ResolveThen("acc", true, func() { approvals++ })
	expectAnswer(t, next, true, "next request")
	if approvals != 1 {
		t.Fatalf("approvals = %d, want 1", approvals)
	}
}

// TestSessionValidatorForgetsRequestsOnClosedLink drops a request sent on a
// link that has since closed: that link answers nothing more, so the next
// request's answer, from the new link, reaches it directly.
func TestSessionValidatorForgetsRequestsOnClosedLink(t *testing.T) {
	v := newSessionValidator(0)
	oldLink := make(chan struct{})
	v.abandon(queuedRequest(t, v, "acc", oldLink))
	close(oldLink)

	next := queuedRequest(t, v, "acc", nil)
	v.Resolve("acc", true)
	expectAnswer(t, next, true, "request on the new link")
}

// TestSessionValidatorRefusesSecondWaitingRequest keeps one waiting client
// per account: a second request is refused until the first is evicted.
func TestSessionValidatorRefusesSecondWaitingRequest(t *testing.T) {
	v := newSessionValidator(0)
	queuedRequest(t, v, "acc", nil)
	if v.enqueue("acc", &authRequest{result: make(chan bool, 1)}) {
		t.Fatal("second waiting request for the account accepted")
	}
	v.Evict("acc")
	queuedRequest(t, v, "acc", nil)
}
