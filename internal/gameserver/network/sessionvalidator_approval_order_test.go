package network

import "testing"

// TestSessionValidatorReportsApprovalBeforeReleasingClient approves a
// waiting request: the approval hook (the login server report) runs while
// the client is still waiting, and only then is the client released.
func TestSessionValidatorReportsApprovalBeforeReleasingClient(t *testing.T) {
	v := newSessionValidator(0)
	req := queuedRequest(t, v, "acc", nil)
	reported := false
	v.ResolveThen("acc", true, func() {
		reported = true
		expectNoAnswer(t, req, "client while the approval is reported")
	})
	if !reported {
		t.Fatal("approval of a waiting client was not reported")
	}
	expectAnswer(t, req, true, "client after the report")
}
