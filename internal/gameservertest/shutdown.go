package gameservertest

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/shutdown"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/rs/zerolog"
)

// loginSyncAccount names the PlayerAuthRequest LoginEntry sends only to
// learn the login server has read everything sent before it; no account
// name can hold its tilde.
const loginSyncAccount = "~login-sync"

// loginLinkSyncWait bounds LoginEntry's wait for the login server's
// answer, a failure guard rather than a pacing delay.
const loginLinkSyncWait = 10 * time.Second

// shutdownHarness is the Boot's login-server listing and timed shutdown:
// the stops the countdown asked for are recorded instead of ending the
// process.
type shutdownHarness struct {
	loginLink *network.LoginLink
	servers   *manager.ServerRegistry
	synced    chan struct{}
	syncMu    sync.Mutex

	mu    sync.Mutex
	stops []bool
}

func newShutdownHarness(servers *manager.ServerRegistry) *shutdownHarness {
	return &shutdownHarness{servers: servers, synced: make(chan struct{}, 1)}
}

// authResponse routes the login server's PlayerAuthResponse: LoginEntry's
// own answer to it, every other to resolve.
func (h *shutdownHarness) authResponse(resolve func(string, bool)) func(string, bool) {
	return func(account string, ok bool) {
		if account == loginSyncAccount {
			h.synced <- struct{}{}
			return
		}
		resolve(account, ok)
	}
}

// wire builds the listing pushed over loginLink, registered under its
// name, and the countdown ticking on q.
func (h *shutdownHarness) wire(loginLink *network.LoginLink, maxPlayers int32, q *queues, notices shutdown.Notifier, log zerolog.Logger) (*network.LoginStatus, *shutdown.Countdown) {
	h.loginLink = loginLink
	status := network.NewLoginStatus(false, maxPlayers, func(st link.ServerStatus) {
		_ = loginLink.SendServerStatus(st)
	})
	status.Registered(loginLink.ServerName)
	countdown := shutdown.New(q.NewQueue("shutdown"), notices, status, func(restart bool) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.stops = append(h.stops, restart)
	}, log)
	return status, countdown
}

// ServerStops returns the stops the timed shutdown asked for, in order:
// true for a restart, false for a shutdown.
func (s *Server) ServerStops() []bool {
	h := s.shutdown
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]bool(nil), h.stops...)
}

// LoginEntry returns the login server's entry for this server, once it has
// read every status the server sent it so far.
func (s *Server) LoginEntry(tb testing.TB) manager.ServerEntry {
	tb.Helper()
	h := s.shutdown
	h.syncMu.Lock()
	defer h.syncMu.Unlock()
	if err := h.loginLink.SendPlayerAuthRequest(link.PlayerAuthRequest{Account: loginSyncAccount}); err != nil {
		tb.Fatalf("send login sync request: %v", err)
	}
	select {
	case <-h.synced:
	case <-time.After(loginLinkSyncWait):
		tb.Fatal("login server did not answer the sync request")
	}
	entry, ok := h.servers.Get(1)
	if !ok {
		tb.Fatal("login server has no entry for server 1")
	}
	return entry
}
