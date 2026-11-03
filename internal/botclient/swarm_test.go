package botclient

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestRunCountsBotsAMidRampEndNeverStarted ends the run during the ramp:
// the first bot starts and fails, the rest are reported as unstarted.
func TestRunCountsBotsAMidRampEndNeverStarted(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens: the first bot's dial is refused

	cfg := DefaultConfig()
	cfg.LoginAddr = addr
	cfg.Bots = 5
	cfg.Rate = 0.01 // the next bot would start after 100s
	cfg.Duration = 50 * time.Millisecond
	cfg.Timeout = time.Second

	report, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.Bots != 5 || report.Started != 1 || report.Entered != 0 {
		t.Fatalf("bots %d, started %d, entered %d; want 5, 1, 0", report.Bots, report.Started, report.Entered)
	}
	if report.Errors["login"]+report.Aborted != 1 {
		t.Fatalf("errors %v, aborted %d; want the one started bot failed", report.Errors, report.Aborted)
	}
}
