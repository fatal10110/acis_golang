package network

import (
	"context"
	"testing"
)

// TestDecideValidationPrioritizesCancellation is the regression test for a
// race where Validate's final select could deliver a login-server rejection
// (sending AuthLoginFail) even though the caller had already canceled: the
// old pre-check only prioritized cancellation that was already done before
// the final select was entered, not one that became ready in the very same
// instant as the result, which Go's select then picks between at random.
// decideValidation is what the final select now consults right as it
// consumes result, so this table exercises the precedence contract directly
// instead of racing goroutines against a select statement.
func TestDecideValidationPrioritizesCancellation(t *testing.T) {
	canceled := context.Canceled

	tests := []struct {
		name       string
		waitCtxErr error
		ok         bool
		want       validationOutcome
	}{
		{"canceled outranks rejection", canceled, false, validationCanceled},
		{"canceled outranks approval", canceled, true, validationCanceled},
		{"rejection without cancellation", nil, false, validationRejected},
		{"approval without cancellation", nil, true, validationApproved},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideValidation(tt.waitCtxErr, tt.ok); got != tt.want {
				t.Fatalf("decideValidation(%v, %v) = %v, want %v", tt.waitCtxErr, tt.ok, got, tt.want)
			}
		})
	}
}
