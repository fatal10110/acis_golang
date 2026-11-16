package lifecycle

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const serverCrashLine = "Server crashed on last session!"

func serverCrashFlag(t *testing.T, srv *gameservertest.Server) string {
	t.Helper()
	var value string
	if err := srv.DB.QueryRow("SELECT value FROM server_memo WHERE var = 'server_crash'").Scan(&value); err != nil {
		t.Fatalf("read server_crash memo: %v", err)
	}
	return value
}

// A running server keeps server_memo's server_crash flag "true" and a clean
// shutdown clears it. A server killed without its shutdown saves leaves the
// flag set, so the next boot logs the crash and leaves the flag set for its
// own session.
func TestServerCrashFlagTracksCleanShutdown(t *testing.T) {
	first := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	if got := serverCrashFlag(t, first); got != "true" {
		t.Fatalf("server_crash after boot = %q, want \"true\"", got)
	}
	if strings.Contains(first.LogText(), serverCrashLine) {
		t.Fatal("first boot on an empty server_memo logged a crash")
	}
	first.Shutdown(t)
	if got := serverCrashFlag(t, first); got != "false" {
		t.Fatalf("server_crash after clean shutdown = %q, want \"false\"", got)
	}

	second := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	if strings.Contains(second.LogText(), serverCrashLine) {
		t.Fatal("boot after a clean shutdown logged a crash")
	}
	if got := serverCrashFlag(t, second); got != "true" {
		t.Fatalf("server_crash after second boot = %q, want \"true\"", got)
	}
	second.Halt(t)
	if got := serverCrashFlag(t, second); got != "true" {
		t.Fatalf("server_crash after a killed session = %q, want \"true\"", got)
	}

	third := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	if !strings.Contains(third.LogText(), serverCrashLine) {
		t.Fatalf("boot after a killed session did not log %q; log:\n%s", serverCrashLine, third.LogText())
	}
	if got := serverCrashFlag(t, third); got != "true" {
		t.Fatalf("server_crash after the crash boot = %q, want \"true\"", got)
	}
}
