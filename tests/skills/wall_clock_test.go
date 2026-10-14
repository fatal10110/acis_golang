package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// skipOnWallClock skips the real-pool pass of a test whose run is a
// timeline of long waits: on the wall clock each wait is slept through,
// while the inline pass runs the same timeline without waiting.
func skipOnWallClock(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	if !srv.DrivesClock() {
		t.Skip("a timeline of long waits, run on the driven clock only")
	}
}
