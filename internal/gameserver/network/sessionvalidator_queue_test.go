package network

import "testing"

func queuedRequest(t *testing.T, v *SessionValidator, account string, linkDone <-chan struct{}) *authRequest {
	t.Helper()
	req := &authRequest{linkDone: linkDone, result: make(chan bool, 1)}
	v.enqueue(account, req)
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

// TestSessionValidatorAnswersEveryWaitingRequestInSendOrder queues several
// clients waiting on one account, one of them abandoned: each live client
// gets its own answer in send order, the abandoned one's answer is dropped
// without running the approval hook, and the hook runs only for the
// approved client.
func TestSessionValidatorAnswersEveryWaitingRequestInSendOrder(t *testing.T) {
	v := newSessionValidator(0)
	first := queuedRequest(t, v, "acc", nil)
	gone := queuedRequest(t, v, "acc", nil)
	v.abandon(gone)
	third := queuedRequest(t, v, "acc", nil)
	owner := queuedRequest(t, v, "acc", nil)

	approvals := 0
	approve := func() { approvals++ }
	v.ResolveThen("acc", false, approve)
	expectAnswer(t, first, false, "first request")
	expectNoAnswer(t, third, "third request after the first answer")

	v.ResolveThen("acc", true, approve)
	expectNoAnswer(t, third, "third request after the abandoned request's answer")
	expectNoAnswer(t, owner, "owner request after the abandoned request's answer")
	if approvals != 0 {
		t.Fatalf("approvals = %d after the abandoned request's answer, want 0", approvals)
	}

	v.ResolveThen("acc", false, approve)
	expectAnswer(t, third, false, "third request")
	expectNoAnswer(t, owner, "owner request after the third answer")

	v.ResolveThen("acc", true, approve)
	expectAnswer(t, owner, true, "owner request")
	if approvals != 1 {
		t.Fatalf("approvals = %d, want 1", approvals)
	}
}
