package netutil

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testFloodConfig() FloodGuardConfig {
	return FloodGuardConfig{
		Enabled:              true,
		FastConnectionLimit:  2,
		NormalConnectionTime: time.Second,
		FastConnectionTime:   300 * time.Millisecond,
		MaxConnectionsPerIP:  3,
	}
}

func TestFloodGuardAdmitsFirstConnection(t *testing.T) {
	g := NewFloodGuard(testFloodConfig(), zerolog.Nop())
	now := time.Now()

	if !g.CanConnect("10.0.0.1", now) {
		t.Fatal("first connection from an unknown IP = false, want true")
	}
}

func TestFloodGuardDropsConnectionInsideFastWindow(t *testing.T) {
	g := NewFloodGuard(testFloodConfig(), zerolog.Nop())
	now := time.Now()

	if !g.CanConnect("10.0.0.1", now) {
		t.Fatal("first connection = false, want true")
	}
	if g.CanConnect("10.0.0.1", now.Add(100*time.Millisecond)) {
		t.Fatal("connection inside FastConnectionTime = true, want false")
	}
	if !g.CanConnect("10.0.0.1", now.Add(400*time.Millisecond)) {
		t.Fatal("connection after FastConnectionTime = false, want true")
	}
}

func TestFloodGuardDropsBurstBeyondFastLimit(t *testing.T) {
	g := NewFloodGuard(testFloodConfig(), zerolog.Nop())
	start := time.Now()
	at := func(ms int) time.Time { return start.Add(time.Duration(ms) * time.Millisecond) }

	if !g.CanConnect("10.0.0.1", at(0)) || !g.CanConnect("10.0.0.1", at(500)) {
		t.Fatal("connections within limits = false, want true")
	}
	if g.CanConnect("10.0.0.1", at(900)) {
		t.Fatal("third fast connection beyond FastConnectionLimit inside NormalConnectionTime = true, want false")
	}
	if g.CanConnect("10.0.0.1", at(1200)) {
		t.Fatal("connection while flagged flooding = true, want false")
	}
	// Rejected attempts refresh the window, so the pace must slow relative
	// to the last (rejected) attempt, matching the reference counters.
	if g.CanConnect("10.0.0.1", at(1700)) {
		t.Fatal("still-fast connection while flagged flooding = true, want false")
	}
	if !g.CanConnect("10.0.0.1", at(2800)) {
		t.Fatal("slowed-down connection after flooding = false, want true")
	}
}

func TestFloodGuardDropsBeyondMaxConnectionsPerIP(t *testing.T) {
	cfg := testFloodConfig()
	g := NewFloodGuard(cfg, zerolog.Nop())
	start := time.Now()

	for i := 0; i < cfg.MaxConnectionsPerIP; i++ {
		at := start.Add(time.Duration(i+1) * cfg.NormalConnectionTime)
		if !g.CanConnect("10.0.0.1", at) {
			t.Fatalf("accepted connection %d = false, want true", i+1)
		}
	}
	if g.CanConnect("10.0.0.1", start.Add(time.Duration(cfg.MaxConnectionsPerIP+1)*cfg.NormalConnectionTime)) {
		t.Fatal("connection beyond MaxConnectionsPerIP = true, want false")
	}
}

func TestFloodGuardReleaseDecaysAttempts(t *testing.T) {
	cfg := testFloodConfig()
	g := NewFloodGuard(cfg, zerolog.Nop())
	start := time.Now()

	for i := range cfg.MaxConnectionsPerIP {
		if !g.CanConnect("10.0.0.1", start.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("accepted connection %d = false, want true", i+1)
		}
	}
	for range cfg.MaxConnectionsPerIP {
		g.Release("10.0.0.1")
	}
	if !g.CanConnect("10.0.0.1", start.Add(time.Duration(cfg.MaxConnectionsPerIP+1)*time.Second)) {
		t.Fatal("connection after full release = false, want true (entry reset)")
	}
}

func TestFloodGuardDisabledAdmitsEverything(t *testing.T) {
	cfg := testFloodConfig()
	cfg.Enabled = false
	g := NewFloodGuard(cfg, zerolog.Nop())
	now := time.Now()

	for i := range 100 {
		if !g.CanConnect("10.0.0.1", now.Add(time.Duration(i)*time.Millisecond)) {
			t.Fatalf("disabled guard rejected connection %d", i+1)
		}
	}
}
