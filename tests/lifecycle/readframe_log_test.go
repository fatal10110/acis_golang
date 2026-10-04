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
