package admin

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The scheduled shutdown and restart tasks (#3836): their start runs
// new Shutdown(600, restart).start(), a countdown thread of its own that is
// not the one //server starts and aborts.

// shutdownTaskStart is when the tasks' commented-out scripts.xml entries
// would start them: a Tuesday, 17:00.
var shutdownTaskStart = time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC)

// bootShutdownTask boots the maintenance world with the task path listed
// as its scripts.xml entry would list it, the task clock a minute before
// its start.
func bootShutdownTask(t *testing.T, path string, ctor func() script.Script) (srv *gameservertest.Server, gm, user *testsupport.ScriptedClient) {
	t.Helper()
	return bootMaintenance(t,
		gameservertest.WithScripts([]script.Listing{{Path: path, Schedule: "WEEKLY", Start: "TUE 17:00:00"}}, script.Catalog{path: ctor}),
		gameservertest.WithScheduledTasks(shutdownTaskStart.Add(-time.Minute)),
	)
}

// startShutdownTask moves the task clock to the tasks' start.
func startShutdownTask(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	srv.ScheduleClock.Advance(shutdownTaskStart.Sub(srv.ScheduleClock.Now()))
	srv.Settle(t)
}

// TestServerRestartTaskCannotBeAborted pins the restart task: nothing is
// said when it starts, 600 not being told at once; each second it warns
// at the countdown's warning times and lists the server down from 60
// seconds left. //server abort does not reach it, and a game master's
// countdown started and aborted meanwhile runs beside it without stopping
// it. At zero it restarts the server.
func TestServerRestartTaskCannotBeAborted(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootShutdownTask(t, "task.ServerRestart", scripttask.ServerRestart)

	startShutdownTask(t, srv)
	assertCountdowns(t, "player at the start", pending(t, user))
	// With no countdown of its own started, abort is the page alone.
	frames := exchange(t, gm, encodeBuildCmd("server abort"))
	if len(frames) != 1 {
		t.Fatalf("//server abort frames = %x, want the page alone", testsupport.FrameOpcodes(frames))
	}
	assertMaintenancePage(t, frames[0], 2, "300", "Auto")

	// 100 seconds in, 500 left: the first warning has been said.
	srv.Advance(t, 100*time.Second)
	assertCountdowns(t, "player at 500", pending(t, user), 540)
	assertCountdowns(t, "game master at 500", pending(t, gm), 540)

	// The game master's own countdown says 450 at once; its abort a second
	// later is told, and the task's countdown carries on.
	frames = exchange(t, gm, encodeBuildCmd("server shutdown 450"))
	if len(frames) != 2 {
		t.Fatalf("//server shutdown 450 frames = %x, want the notice then the page", testsupport.FrameOpcodes(frames))
	}
	assertCountdowns(t, "game master's countdown start", frames[:1], 450)
	assertCountdowns(t, "player at the game master's start", pending(t, user), 450)
	srv.Advance(t, time.Second)
	frames = exchange(t, gm, encodeBuildCmd("server abort"))
	if len(frames) != 2 {
		t.Fatalf("//server abort frames = %x, want the notice then the page", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", "Server aborted shutting down process and continues normal operation.")

	srv.Advance(t, 438*time.Second)
	assertCountdowns(t, "player at 61", pending(t, user), 480, 420, 360, 300, 240, 180, 120)
	assertLogin(t, srv, link.ServerTypeAuto, 300)
	srv.Advance(t, time.Second)
	assertCountdowns(t, "player at 60", pending(t, user), 60)
	assertLogin(t, srv, link.ServerTypeDown, 300)
	srv.Advance(t, 59*time.Second)
	assertCountdowns(t, "player at 1", pending(t, user), 30, 10, 5, 4, 3, 2, 1)
	assertNoStops(t, srv, "with a second left")
	srv.Advance(t, time.Second)
	if stops := srv.ServerStops(); !slices.Equal(stops, []bool{true}) {
		t.Fatalf("stops = %v, want one restart", stops)
	}
}

// TestServerShutdownTaskStopsWithoutRestart pins the shutdown task's mode:
// the same unannounced 600 second countdown, which at zero shuts the
// server down without a restart.
func TestServerShutdownTaskStopsWithoutRestart(t *testing.T) {
	t.Parallel()
	srv, _, user := bootShutdownTask(t, "task.ServerShutdown", scripttask.ServerShutdown)

	startShutdownTask(t, srv)
	assertCountdowns(t, "player at the start", pending(t, user))
	srv.Advance(t, 60*time.Second)
	assertCountdowns(t, "player after a minute", pending(t, user), 540)
	srv.Advance(t, 539*time.Second)
	assertNoStops(t, srv, "with a second left")
	srv.Advance(t, time.Second)
	if stops := srv.ServerStops(); !slices.Equal(stops, []bool{false}) {
		t.Fatalf("stops = %v, want one shutdown", stops)
	}
}
