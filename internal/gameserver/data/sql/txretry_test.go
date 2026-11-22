package sql

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestLockConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"deadlock", &mysql.MySQLError{Number: 1213}, true},
		{"lock wait timeout", &mysql.MySQLError{Number: 1205}, true},
		{"wrapped deadlock", fmt.Errorf("save: %w", &mysql.MySQLError{Number: 1213}), true},
		{"duplicate key", &mysql.MySQLError{Number: 1062}, false},
		{"data too long", &mysql.MySQLError{Number: 1406}, false},
		{"not a database error", errors.New("boom"), false},
		{"nil", nil, false},
	} {
		if got := lockConflict(tc.err); got != tc.want {
			t.Errorf("lockConflict(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRetryLockConflict(t *testing.T) {
	deadlock := fmt.Errorf("save: %w", &mysql.MySQLError{Number: 1213})
	other := errors.New("insert failed")

	// failing returns errs[i] on call i and nil once they run out.
	failing := func(calls *int, errs ...error) func() error {
		return func() error {
			i := *calls
			*calls++
			if i < len(errs) {
				return errs[i]
			}
			return nil
		}
	}

	t.Run("conflicts then success", func(t *testing.T) {
		calls := 0
		err := retryLockConflict(context.Background(), failing(&calls, deadlock, deadlock))
		if err != nil || calls != 3 {
			t.Fatalf("retryLockConflict() = %v after %d calls, want nil after 3", err, calls)
		}
	})
	t.Run("bounded", func(t *testing.T) {
		calls := 0
		errs := make([]error, txAttempts+1)
		for i := range errs {
			errs[i] = deadlock
		}
		err := retryLockConflict(context.Background(), failing(&calls, errs...))
		if !errors.Is(err, deadlock) || calls != txAttempts {
			t.Fatalf("retryLockConflict() = %v after %d calls, want the conflict after %d", err, calls, txAttempts)
		}
	})
	t.Run("other error not retried", func(t *testing.T) {
		calls := 0
		err := retryLockConflict(context.Background(), failing(&calls, other))
		if !errors.Is(err, other) || calls != 1 {
			t.Fatalf("retryLockConflict() = %v after %d calls, want %v after 1", err, calls, other)
		}
	})
	t.Run("ended context not retried", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := retryLockConflict(ctx, failing(&calls, deadlock))
		if !errors.Is(err, deadlock) || calls != 1 {
			t.Fatalf("retryLockConflict() = %v after %d calls, want the conflict after 1", err, calls)
		}
	})
}
