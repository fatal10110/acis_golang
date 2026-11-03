package gameservertest

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
)

// loginSide is the login server state Boot's GS-LS acceptor runs on, kept so
// a suite can also open the client-facing login port on it.
type loginSide struct {
	accounts *loginsql.AccountStore
	servers  *manager.ServerRegistry
	sessions *manager.SessionStore
	bans     *manager.IPBanList
	roster   *loginserver.LinkRoster
}

// sharedLoginKeys is the client-facing RSA key pool every login port in the
// process shares; generating one costs ten RSA key generations.
var sharedLoginKeys = sync.OnceValues(manager.NewLoginKeyPool)

// StartLoginClients opens the login server's client-facing port on the same
// account store, server registry and session store the game server's login
// link uses, with AutoCreateAccounts and ShowLicence on (the shipped
// defaults), and reports server 1 as up. It returns the address an external
// client logs in at, so a suite can drive the whole login-to-world path over
// real sockets. The ServerList still advertises the registered port 7777;
// clients dial Addr for the game server.
func (s *Server) StartLoginClients(t *testing.T) string {
	t.Helper()
	keys, err := sharedLoginKeys()
	if err != nil {
		t.Fatalf("NewLoginKeyPool: %v", err)
	}
	auto := link.ServerTypeAuto
	if _, ok := s.login.servers.ApplyStatus(1, link.ServerStatus{Status: &auto}); !ok {
		t.Fatal("game server 1 is not registered with the login server")
	}
	clients := loginserver.NewClientLink(s.login.accounts, s.login.servers, s.login.sessions, s.login.bans, keys, s.login.roster,
		true, true, 5, 10*time.Minute, zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cleanup(t, cancel)
	// Serve returns once cleanup cancels ctx; nothing waits on its error.
	go func() { _ = clients.Serve(ctx, ln) }()
	return ln.Addr().String()
}
