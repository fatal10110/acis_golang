package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// needsDrivenClock skips the test on the real pool, where Advance sleeps
// while a timer is a callback of its own: a test that pins a state a
// millisecond before a deadline, or exactly on it, races that callback there.
// So does one that pins it a fraction of a second before (an advance of
// 4900 ms against a 5 s timer), or arranges its draws before a timer the NPC
// armed as it appeared: the wall clock also counts the test's own work
// between the arming and the check, which a loaded run stretches past that
// margin. The inline pass holds the boundary.
func needsDrivenClock(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	if !srv.DrivesClock() {
		t.Skip("pins a state at a timer's deadline, which the wall clock cannot hold; the driven clock covers it")
	}
}
