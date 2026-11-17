package network

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestMaintainReturnsOnlyAfterTheLinkEnds cancels maintain while a link is
// up: maintain closes the link and returns only once its read loop has
// ended, so nothing of the link outlives the caller's shutdown.
func TestMaintainReturnsOnlyAfterTheLinkEnds(t *testing.T) {
	addr, servers, _ := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	links := make(chan *LoginLink, 1)
	auth := func() LoginServerAuth {
		return LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	}
	never := func(time.Duration) <-chan time.Time { return nil }
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		maintain(ctx, addr, auth, LoginLinkHandlers{}, DefaultReconnectDelay, never, func(l *LoginLink) { links <- l }, zerolog.Nop())
	}()

	link := within(t, links, "the link")
	cancel()
	within(t, stopped, "maintain to return")
	select {
	case <-link.Done():
	default:
		t.Fatal("maintain returned while the link's read loop was still running")
	}
}
