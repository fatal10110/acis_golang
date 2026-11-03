package admin

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// serverUsage is //server's answer to a missing or malformed number.
const serverUsage = "Usage: //server <shutdown|restart|abort|gmonly|all|max>."

// bootMaintenance boots the game master with the maintenance page, entered
// in the world, and a second player beside it. extra joins the boot
// options.
func bootMaintenance(t *testing.T, extra ...gameservertest.Option) (srv *gameservertest.Server, gm, user *testsupport.ScriptedClient) {
	t.Helper()
	srv, _ = bootAdmin(t, adminLevel, append([]gameservertest.Option{gameservertest.WithHTMLPages(shippedAdminPages(t, "maintenance.htm"))}, extra...)...)
	if !srv.DrivesClock() {
		t.Skip("counting the countdown's seconds needs the driven clock")
	}
	gm = srv.Client
	enterWorld(t, gm)
	user, _ = addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	return srv, gm, user
}

// countdowns returns the seconds of every countdown notice among frames,
// in order, skipping every other frame.
func countdowns(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var seconds []int32
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != serverpackets.SystemMessageServerDownInS1Seconds {
			continue
		}
		if n, typ := r.ReadInt32(), r.ReadInt32(); n != 1 || typ != serverpackets.SystemMessageParamNumber {
			t.Fatalf("countdown notice %x: %d params of type %d, want one number", frame, n, typ)
		}
		seconds = append(seconds, r.ReadInt32())
		if err := r.Err(); err != nil || r.Remaining() != 0 {
			t.Fatalf("countdown notice %x: read %v, %d bytes left", frame, err, r.Remaining())
		}
	}
	return seconds
}

// assertCountdowns requires the countdown notices among frames to be want.
func assertCountdowns(t *testing.T, who string, frames [][]byte, want ...int32) {
	t.Helper()
	if got := countdowns(t, frames); !slices.Equal(got, want) {
		t.Fatalf("%s countdown notices = %v, want %v", who, got, want)
	}
}

// assertMaintenancePage requires frame to be the maintenance page showing
// the server, Bartz, with online players, capped at maxPlayers and listed
// under status. Memory in use and the in-game time are any number and any
// HH:MM.
func assertMaintenancePage(t *testing.T, frame []byte, online int, maxPlayers, status string) {
	t.Helper()
	// The page cache ends every line, the last one too, with a bare line
	// break.
	template := strings.ReplaceAll(shippedAdminPages(t, "maintenance.htm")["admin/maintenance.htm"], "\r\n", "\n") + "\n"
	pattern := strings.NewReplacer(
		"%count%", strconv.Itoa(online),
		"%server_name%", "Bartz",
		"%max_players%", regexp.QuoteMeta(maxPlayers),
		"%status%", regexp.QuoteMeta(status),
		"%used%", `\d+`,
		"%time%", `\d\d:\d\d`,
	).Replace(regexp.QuoteMeta(template))
	if body := htmlBody(t, frame); !regexp.MustCompile(`^` + pattern + `$`).MatchString(body) {
		t.Fatalf("maintenance page = %q, want the shipped page with %d online, cap %s, status %s", body, online, maxPlayers, status)
	}
}

// assertLogin requires the login server to list the server under status
// with a cap of maxPlayers.
func assertLogin(t *testing.T, srv *gameservertest.Server, status link.ServerType, maxPlayers int32) {
	t.Helper()
	if e := srv.LoginEntry(t); e.Status != status || e.MaxPlayers != maxPlayers {
		t.Fatalf("login server lists %v capped at %d, want %v capped at %d", e.Status, e.MaxPlayers, status, maxPlayers)
	}
}

// TestServerShutdownCountdown pins //server shutdown <seconds> (AdminMaintenance
// and Shutdown.startShutdown/countdown): a count that is not one of the
// countdown's warnings is told every player at once, then each second the
// countdown warns at 60, 30, 10, 5, 4, 3, 2 and 1 seconds left, lists the
// server down from 60 seconds left, and a second after the last warning
// stops the server without a restart.
func TestServerShutdownCountdown(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootMaintenance(t)

	frames := exchange(t, gm, encodeBuildCmd("server shutdown 65"))
	if len(frames) != 2 {
		t.Fatalf("//server shutdown 65 frames = %x, want the countdown notice then the page", testsupport.FrameOpcodes(frames))
	}
	assertCountdowns(t, "game master", frames[:1], 65)
	assertMaintenancePage(t, frames[1], 2, "300", "Auto")
	assertCountdowns(t, "player", pending(t, user), 65)
	assertLogin(t, srv, link.ServerTypeAuto, 300)

	warned := map[int32]bool{60: true, 30: true, 10: true, 5: true, 4: true, 3: true, 2: true, 1: true}
	for left := int32(64); left >= 1; left-- {
		srv.Advance(t, time.Second)
		if warned[left] {
			assertCountdowns(t, "player at "+strconv.Itoa(int(left)), pending(t, user), left)
		} else {
			assertCountdowns(t, "player at "+strconv.Itoa(int(left)), pending(t, user))
		}
		switch left {
		case 61:
			assertLogin(t, srv, link.ServerTypeAuto, 300)
		case 60:
			assertLogin(t, srv, link.ServerTypeDown, 300)
		}
		if stops := srv.ServerStops(); len(stops) != 0 {
			t.Fatalf("server stopped with %d seconds left: %v", left, stops)
		}
	}
	srv.Advance(t, time.Second)
	if stops := srv.ServerStops(); !slices.Equal(stops, []bool{false}) {
		t.Fatalf("stops = %v, want one shutdown", stops)
	}
}

// TestServerRestartAbort pins //server restart and //server abort
// (Shutdown.abort and the countdown's abort check): abort tells every
// player the restart is off, the countdown notices on its next tick and
// lists the server open again, and nothing more is warned or stopped. Abort
// is said again on every later abort.
func TestServerRestartAbort(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootMaintenance(t)

	assertCountdowns(t, "game master", exchange(t, gm, encodeBuildCmd("server restart 62")), 62)
	assertCountdowns(t, "player", pending(t, user), 62)
	srv.Advance(t, time.Second)
	srv.Advance(t, time.Second)
	assertCountdowns(t, "player", pending(t, user), 60)
	assertCountdowns(t, "game master", pending(t, gm), 60)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	const aborted = "Server aborted restarting process and continues normal operation."
	frames := exchange(t, gm, encodeBuildCmd("server abort"))
	if len(frames) != 2 {
		t.Fatalf("//server abort frames = %x, want the notice then the page", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", aborted)
	// The countdown notices the abort on its next tick.
	assertMaintenancePage(t, frames[1], 2, "300", "Down")
	frames = pending(t, user)
	if len(frames) != 1 {
		t.Fatalf("player frames after abort = %x, want the notice", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", aborted)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	srv.Advance(t, time.Second)
	assertLogin(t, srv, link.ServerTypeAuto, 300)
	for range 70 {
		srv.Advance(t, time.Second)
	}
	assertCountdowns(t, "player", pending(t, user))
	if stops := srv.ServerStops(); len(stops) != 0 {
		t.Fatalf("stops after abort = %v, want none", stops)
	}

	frames = exchange(t, gm, encodeBuildCmd("server abort"))
	if len(frames) != 2 {
		t.Fatalf("second //server abort frames = %x, want the notice then the page", testsupport.FrameOpcodes(frames))
	}
	assertSay(t, frames[0], 0, sayAnnouncement, "", aborted)
	assertMaintenancePage(t, frames[1], 2, "300", "Auto")
}

// TestServerMaintenanceCommands pins the rest of //server: the page alone
// with no argument, an unknown one, or abort before any countdown; the
// usage line for a missing or malformed number; and gmonly, all and max,
// each told to the game master and pushed to the login server.
func TestServerMaintenanceCommands(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootMaintenance(t)

	for _, cmd := range []string{"server", "server bogus", "server abort"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want the page alone", cmd, testsupport.FrameOpcodes(frames))
		}
		assertMaintenancePage(t, frames[0], 2, "300", "Auto")
	}
	for _, cmd := range []string{"server shutdown", "server restart soon", "server max", "server max 1e3"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 2 {
			t.Fatalf("//%s frames = %x, want the usage line then the page", cmd, testsupport.FrameOpcodes(frames))
		}
		assertTexts(t, frames[:1], serverUsage)
		assertMaintenancePage(t, frames[1], 2, "300", "Auto")
	}
	if stops := srv.ServerStops(); len(stops) != 0 {
		t.Fatalf("stops = %v, want none", stops)
	}

	steps := []struct {
		cmd, text, maxPlayers, status string
		listed                        link.ServerType
		capped                        int32
	}{
		{"server max 500", "Server maximum player amount is set to 500.", "500", "Auto", link.ServerTypeAuto, 500},
		{"server gmonly", "Server is now set as GMonly.", "500", "Gm Only", link.ServerTypeGMOnly, 500},
		{"server all", "Server isn't set as GMonly anymore.", "500", "Auto", link.ServerTypeAuto, 500},
	}
	for _, step := range steps {
		frames := exchange(t, gm, encodeBuildCmd(step.cmd))
		if len(frames) != 2 {
			t.Fatalf("//%s frames = %x, want the answer then the page", step.cmd, testsupport.FrameOpcodes(frames))
		}
		assertTexts(t, frames[:1], step.text)
		assertMaintenancePage(t, frames[1], 2, step.maxPlayers, step.status)
		assertLogin(t, srv, step.listed, step.capped)
	}
}

// TestServerShutdownRestoresGMOnly pins the countdown's corner cases: an
// abort on a server reserved for game masters lists it reserved again, a
// count that is itself a warning is told by the countdown's first tick
// rather than at once, and a negative count is told as given and stops at
// the first tick.
func TestServerShutdownRestoresGMOnly(t *testing.T) {
	t.Parallel()
	srv, gm, user := bootMaintenance(t)

	exchange(t, gm, encodeBuildCmd("server gmonly"))
	frames := exchange(t, gm, encodeBuildCmd("server shutdown 3"))
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("//server shutdown 3 frames = %x, want the page then the first tick's notice", testsupport.FrameOpcodes(frames))
	}
	assertMaintenancePage(t, frames[0], 2, "300", "Gm Only")
	assertCountdowns(t, "game master", frames[1:], 3)
	assertCountdowns(t, "player", pending(t, user), 3)
	assertLogin(t, srv, link.ServerTypeDown, 300)

	exchange(t, gm, encodeBuildCmd("server abort"))
	srv.Advance(t, time.Second)
	assertLogin(t, srv, link.ServerTypeGMOnly, 300)
	assertCountdowns(t, "player", pending(t, user))

	frames = exchange(t, gm, encodeBuildCmd("server restart -5"))
	assertCountdowns(t, "game master", frames, -5)
	assertCountdowns(t, "player", pending(t, user), -5)
	if stops := srv.ServerStops(); !slices.Equal(stops, []bool{true}) {
		t.Fatalf("stops = %v, want one restart", stops)
	}
}
