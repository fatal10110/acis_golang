package lifecycle

import (
	"net"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestReadFrameEOFLoggedOnlyAfterAuth pins the EOF log split: a connection
// that hangs up before authenticating (a port probe dialing and closing)
// leaves no "Read frame" line, while an authenticated client's disconnect is
// still logged.
func TestReadFrameEOFLoggedOnlyAfterAuth(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	readFrameLines := func() int { return strings.Count(srv.LogText(), `"Read frame"`) }

	// A raw client waits on the wall clock: half-close so the server reads
	// EOF, then wait for its own close. The loop logs before it returns and
	// the listener closes the socket after, so the log is final here.
	probe := testsupport.Dial(t, srv.Addr())
	if err := probe.Conn().(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}
	probe.ExpectClosed()
	if n := readFrameLines(); n != 0 {
		t.Fatalf("pre-auth hang-up logged %d Read frame lines, want 0", n)
	}

	// Boot leaves Client authenticated at character select. It reads on the
	// driven clock, which cannot see a hang-up the server has not answered,
	// so wait for the log line itself.
	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "post-auth Read frame log", func() bool { return readFrameLines() > 0 })
	if n := readFrameLines(); n != 1 {
		t.Fatalf("post-auth hang-up logged %d Read frame lines, want 1", n)
	}
}

// TestReadFramePartialFrameLogLevels pins the mid-frame hang-up: before
// auth a peer that closes part-way through a frame (a scanner's "GET /" read
// as a frame header) leaves no "Read frame" line; after auth the same
// hang-up is an ordinary disconnect, logged at debug with the peer address.
func TestReadFramePartialFrameLogLevels(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	readFrameLines := func() []string {
		var out []string
		for _, line := range strings.Split(srv.LogText(), "\n") {
			if strings.Contains(line, `"Read frame"`) {
				out = append(out, line)
			}
		}
		return out
	}
	hangUpMidFrame := func(conn net.Conn, partial []byte) {
		t.Helper()
		if _, err := conn.Write(partial); err != nil {
			t.Fatalf("write partial frame: %v", err)
		}
		if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
			t.Fatalf("CloseWrite: %v", err)
		}
	}

	for name, partial := range map[string][]byte{
		"one header byte":  {0x47},
		"short payload":    []byte("GET / HTTP/1.0"),
		"one payload byte": {0x10, 0x00, 0x01},
	} {
		probe := testsupport.Dial(t, srv.Addr())
		hangUpMidFrame(probe.Conn(), partial)
		probe.ExpectClosed()
		if lines := readFrameLines(); len(lines) != 0 {
			t.Fatalf("pre-auth %s hang-up logged %q, want nothing", name, lines)
		}
	}

	// A 16-byte frame announced, three bytes sent.
	hangUpMidFrame(srv.Client.Conn(), []byte{0x10, 0x00, 0x01, 0x02, 0x03})
	srv.AdvanceUntil(t, "post-auth Read frame log", func() bool { return len(readFrameLines()) > 0 })
	lines := readFrameLines()
	if len(lines) != 1 {
		t.Fatalf("post-auth mid-frame hang-up logged %q, want one line", lines)
	}
	for _, want := range []string{`"level":"debug"`, `"error":"unexpected EOF"`, `"addr":"127.0.0.1"`} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("post-auth Read frame line %s lacks %s", lines[0], want)
		}
	}
}

// TestReadFrameResetLogLevels pins the reset hang-up: a peer that closes
// with a reset (SO_LINGER 0, a killed client) hung up like one that closed
// cleanly. Before auth it leaves no "Read frame" line; in the world it is an
// ordinary disconnect, logged at debug, and nothing about it logs an error
// or a warn-level write failure.
func TestReadFrameResetLogLevels(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
	)
	logLines := func(substr string) []string {
		var out []string
		for _, line := range strings.Split(srv.LogText(), "\n") {
			if strings.Contains(line, substr) {
				out = append(out, line)
			}
		}
		return out
	}
	reset := func(conn net.Conn) {
		t.Helper()
		tcp := conn.(*net.TCPConn)
		if err := tcp.SetLinger(0); err != nil {
			t.Fatalf("SetLinger: %v", err)
		}
		if err := tcp.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	probe := testsupport.Dial(t, srv.Addr())
	reset(probe.Conn())
	srv.AwaitConnectionEnded(t, probe)
	if lines := logLines(`"Read frame"`); len(lines) != 0 {
		t.Fatalf("pre-auth reset logged %q, want nothing", lines)
	}

	startInWorld(t, srv.Client)
	reset(srv.Client.Conn())
	srv.AdvanceUntil(t, "in-world Read frame log", func() bool { return len(logLines(`"Read frame"`)) > 0 })
	lines := logLines(`"Read frame"`)
	if len(lines) != 1 {
		t.Fatalf("in-world reset logged %q, want one line", lines)
	}
	for _, want := range []string{`"level":"debug"`, `connection reset by peer`, `"addr":"127.0.0.1"`} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("in-world Read frame line %s lacks %s", lines[0], want)
		}
	}
	for _, line := range logLines(`"game connection write failed, closing"`) {
		if !strings.Contains(line, `"level":"debug"`) {
			t.Fatalf("write to the reset peer logged %s, want debug", line)
		}
	}
	if lines := logLines(`"level":"error"`); len(lines) != 0 {
		t.Fatalf("reset logged errors: %q", lines)
	}
}

// TestRejectedPreAuthFrameLogsAddr pins the address on the two warnings a
// non-client talking to the game port draws: a ProtocolVersion too short to
// decode and an opcode refused before auth.
func TestRejectedPreAuthFrameLogsAddr(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	for msg, frame := range map[string][]byte{
		`"game client"`:   {0x03, 0x00, 0x00},
		`"Accept opcode"`: {0x03, 0x00, 0x47},
	} {
		probe := testsupport.Dial(t, srv.Addr())
		if _, err := probe.Conn().Write(frame); err != nil {
			t.Fatalf("write %s frame: %v", msg, err)
		}
		srv.AwaitConnectionEnded(t, probe)
		var found bool
		for _, line := range strings.Split(srv.LogText(), "\n") {
			if !strings.Contains(line, msg) {
				continue
			}
			found = true
			if !strings.Contains(line, `"level":"warn"`) || !strings.Contains(line, `"addr":"127.0.0.1"`) {
				t.Fatalf("%s line %s, want a warn with the peer address", msg, line)
			}
		}
		if !found {
			t.Fatalf("no %s line logged; log:\n%s", msg, srv.LogText())
		}
	}
}
