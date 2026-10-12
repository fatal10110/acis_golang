package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// assertNoStops requires the timed shutdown to have asked for no stop.
func assertNoStops(t *testing.T, srv *gameservertest.Server, when string) {
	t.Helper()
	if stops := srv.ServerStops(); len(stops) != 0 {
		t.Fatalf("stops %s = %v, want none", when, stops)
	}
}

// TestServerRestartReplacesRunningCountdown pins a new countdown started
// while one is still running (Shutdown.startShutdown aborting the running
// counter): the old countdown warns no more, gives the login server its
// listing back on its next tick, and never stops the server; only the new
// countdown's warnings follow.
func TestServerRestartReplacesRunningCountdown(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootMaintenance(t)

	assertCountdowns(t, "game master", exchange(t, gm, encodeBuildCmd("server shutdown 62")), 62)
	assertCountdowns(t, "player", pending(t, user), 62)
	srv.Advance(t, time.Second)
	srv.Advance(t, time.Second)
	assertCountdowns(t, "player", pending(t, user), 60)
	assertCountdowns(t, "game master", pending(t, gm), 60)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	// 300 is one of the countdown's warnings: the new countdown's first
	// tick tells it, after the page.
	frames := exchange(t, gm, encodeBuildCmd("server restart 300"))
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("//server restart 300 frames = %x, want the page then the first tick's notice", testsupport.FrameOpcodes(frames))
	}
	assertMaintenancePage(t, frames[0], 2, "300", "Down")
	assertCountdowns(t, "game master", frames[1:], 300)
	assertCountdowns(t, "player", pending(t, user), 300)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	// The old countdown notices it was replaced on its next tick.
	srv.Advance(t, time.Second)
	assertLogin(t, srv, link.ServerTypeAuto, 300)
	assertCountdowns(t, "player", pending(t, user))

	// Past the second the old countdown would have stopped the server, up
	// to the new countdown's next warning.
	for range 60 {
		srv.Advance(t, time.Second)
		assertNoStops(t, srv, "after the countdown was replaced")
	}
	assertCountdowns(t, "player", pending(t, user), 240)
	assertCountdowns(t, "game master", pending(t, gm), 240)
	assertLogin(t, srv, link.ServerTypeAuto, 300)
}

// TestServerAbortInLastSecond pins an abort after the countdown's last
// warning (the countdown loop ending with nothing left to count while
// aborted): the server is neither stopped nor listed open again.
func TestServerAbortInLastSecond(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootMaintenance(t)

	frames := exchange(t, gm, encodeBuildCmd("server shutdown 2"))
	assertCountdowns(t, "game master", frames, 2)
	assertCountdowns(t, "player", pending(t, user), 2)
	srv.Advance(t, time.Second)
	assertCountdowns(t, "player", pending(t, user), 1)
	assertCountdowns(t, "game master", pending(t, gm), 1)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	const aborted = "Server aborted shutting down process and continues normal operation."
	frames = exchange(t, gm, encodeBuildCmd("server abort"))
	if len(frames) != 2 {
		t.Fatalf("//server abort frames = %x, want the notice then the page", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", aborted)

	for range 3 {
		srv.Advance(t, time.Second)
		assertNoStops(t, srv, "after an abort in the last second")
	}
	assertLogin(t, srv, link.ServerTypeDown, 300)
	frames = pending(t, user)
	if len(frames) != 1 {
		t.Fatalf("player frames after abort = %x, want the notice alone", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", aborted)
}
