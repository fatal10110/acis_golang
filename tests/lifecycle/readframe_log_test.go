package lifecycle

import (
	"net"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// hangUp half-closes c so the server reads EOF, then waits for the server's
// own close. The client loop logs before it returns and the listener closes
// the connection after, so once this returns the EOF log line is either
// written or never will be.
func hangUp(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	if err := c.Conn().(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}
	c.ExpectClosed()
}

// TestReadFrameEOFLoggedOnlyAfterAuth pins the EOF log split: a connection
// that hangs up before authenticating (a port probe dialing and closing)
// leaves no "Read frame" line, while an authenticated client's disconnect is
// still logged.
func TestReadFrameEOFLoggedOnlyAfterAuth(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	readFrameLines := func() int { return strings.Count(srv.LogText(), `"Read frame"`) }

	hangUp(t, testsupport.Dial(t, srv.Addr()))
	if n := readFrameLines(); n != 0 {
		t.Fatalf("pre-auth hang-up logged %d Read frame lines, want 0", n)
	}

	hangUp(t, srv.Client) // Boot leaves Client authenticated at character select
	if n := readFrameLines(); n != 1 {
		t.Fatalf("post-auth hang-up logged %d Read frame lines, want 1", n)
	}
}
