package admin

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
)

// TestServerRestartInLastMinuteKeepsListingClosed pins a countdown replaced
// inside the last minute by one that is itself inside its last minute: the
// replaced countdown's last tick leaves the listing closed instead of
// listing the server open for a second until the new countdown's next tick
// closes it again, and the new countdown runs to its restart.
func TestServerRestartInLastMinuteKeepsListingClosed(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootMaintenance(t)

	assertCountdowns(t, "game master", exchange(t, gm, encodeBuildCmd("server shutdown 62")), 62)
	srv.Advance(t, time.Second)
	srv.Advance(t, time.Second)
	assertCountdowns(t, "player", pending(t, user), 62, 60)
	assertCountdowns(t, "game master", pending(t, gm), 60)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	assertCountdowns(t, "game master", exchange(t, gm, encodeBuildCmd("server restart 30")), 30)
	assertLogin(t, srv, link.ServerTypeDown, 300)
	before := len(srv.ListedStatuses())

	// The replaced countdown notices it on this tick; the new one is
	// already closing logins.
	srv.Advance(t, time.Second)
	assertLogin(t, srv, link.ServerTypeDown, 300)
	if sent := srv.ListedStatuses()[before:]; len(sent) != 0 {
		t.Fatalf("statuses sent after the replaced countdown's last tick = %v, want none", sent)
	}

	for range 29 {
		srv.Advance(t, time.Second)
	}
	if stops := srv.ServerStops(); !slices.Equal(stops, []bool{true}) {
		t.Fatalf("stops = %v, want one restart", stops)
	}
	if sent := srv.ListedStatuses()[before:]; slices.Contains(sent, link.ServerTypeAuto) {
		t.Fatalf("statuses sent after the restart = %v, want the listing never reopened", sent)
	}
}
