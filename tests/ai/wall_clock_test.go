package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// needsDrivenClock skips the test on the real pool, where Advance sleeps
// while a timer is a callback of its own: a test that pins a state a
// millisecond before a deadline, or exactly on it, races that callback there.
// The inline pass holds the boundary.
func needsDrivenClock(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	if !srv.DrivesClock() {
		t.Skip("pins a state at a timer's deadline, which the wall clock cannot hold; the driven clock covers it")
	}
}
