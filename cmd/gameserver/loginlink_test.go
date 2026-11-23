package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// startLoginServer runs a real login server's game-server link acceptor
// that admits new servers under ids 1 and 2, and returns its address and
// server registry.
func startLoginServer(t *testing.T) (string, *manager.ServerRegistry) {
	t.Helper()
	namesPath := filepath.Join(t.TempDir(), "serverNames.xml")
	if err := os.WriteFile(namesPath, []byte(`<?xml version='1.0'?><list>
		<server id="1" name="Bartz" />
		<server id="2" name="Sieghardt" />
	</list>`), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := manager.LoadServerNames(namesPath)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := manager.NewRSAKeyPool()
	if err != nil {
		t.Fatal(err)
	}
	servers := manager.NewServerRegistry()
	gsLink := loginserver.NewGameServerLink(servers, names, keys, manager.NewSessionStore(), manager.NewIPBanList(zerolog.Nop()),
		nil, acceptRegistrations{}, true, nil, loginserver.NewLinkRoster(), zerolog.Nop())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = gsLink.Serve(ctx, ln) }()
	return ln.Addr().String(), servers
}

// acceptRegistrations stores every game server registration nowhere and
// reports success, so the login server accepts the link.
type acceptRegistrations struct{}

func (acceptRegistrations) CreateGameServer(context.Context, model.GameServer) error { return nil }

// syncLink waits until the login server has read everything sent on l so
// far: it answers a session check only after the messages before it.
func syncLink(t *testing.T, l *network.LoginLink, answers <-chan string) {
	t.Helper()
	if err := l.SendPlayerAuthRequest(link.PlayerAuthRequest{Account: "sync"}); err != nil {
		t.Fatalf("send sync request: %v", err)
	}
	select {
	case <-answers:
	case <-time.After(10 * time.Second):
		t.Fatal("login server did not answer the sync request")
	}
}

// TestOnLoginLinkSavesAssignedServerID registers with a desired id another
// server's key already holds: the login server assigns the next free id,
// the link-up writes it with this server's key to the hexid file, and the
// next boot's registration asks for that id.
func TestOnLoginLinkSavesAssignedServerID(t *testing.T) {
	addr, servers := startLoginServer(t)
	servers.Register(1, []byte{0x55, 0x66, 0x77, 0x08})
	key := []byte{0x11, 0x22, 0x33, 0x44}

	l, err := network.DialLoginLink(context.Background(), addr,
		network.LoginServerAuth{ServerID: 1, AcceptAlternateID: true, HexID: key, HostName: "*", Port: 7777, MaxPlayers: 300},
		network.LoginLinkHandlers{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()
	if l.ServerID != 2 {
		t.Fatalf("assigned ServerID = %d, want 2", l.ServerID)
	}

	paths := gameServerPaths{HexIDPath: filepath.Join(t.TempDir(), "hexid.txt")}
	links := provideLoginLinkState()
	status := network.NewLoginStatus(false, 300, func(link.ServerStatus) {})
	onLoginLink(links, status, world.New(), paths.HexIDPath, key, zerolog.Nop())(l)
	if links.get() != l {
		t.Fatal("link not published")
	}

	hexProps, err := loadHexIDProperties(paths)
	if err != nil {
		t.Fatalf("load written hexid file: %v", err)
	}
	serverProps, err := config.ParseString("")
	if err != nil {
		t.Fatal(err)
	}
	next, err := gameServerConfigFromProperties(paths, serverProps, hexProps.Props)
	if err != nil {
		t.Fatalf("next boot's config: %v", err)
	}
	if next.Auth.ServerID != 2 || !bytes.Equal(next.Auth.HexID, key) {
		t.Fatalf("next boot registers as id %d key %x, want id 2 key %x", next.Auth.ServerID, next.Auth.HexID, key)
	}
}

// TestOnLoginLinkKeepsLinkWhenHexIDWriteFails links up with an unwritable
// hexid path: the failure is logged and the link stays up and usable.
func TestOnLoginLinkKeepsLinkWhenHexIDWriteFails(t *testing.T) {
	addr, servers := startLoginServer(t)
	key := []byte{0x11, 0x22, 0x33, 0x44}
	servers.Register(1, key)
	answers := make(chan string, 1)
	l, err := network.DialLoginLink(context.Background(), addr,
		network.LoginServerAuth{ServerID: 1, HexID: key, HostName: "*", Port: 7777, MaxPlayers: 300},
		network.LoginLinkHandlers{PlayerAuthResponse: func(account string, _ bool) { answers <- account }}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()

	links := provideLoginLinkState()
	status := network.NewLoginStatus(false, 300, func(link.ServerStatus) {})
	onLoginLink(links, status, world.New(), filepath.Join(t.TempDir(), "missing", "hexid.txt"), key, zerolog.Nop())(l)

	syncLink(t, l, answers)
	select {
	case <-l.Done():
		t.Fatal("link ended after the failed hexid write")
	default:
	}
	if links.get() != l {
		t.Fatal("link not published")
	}
}

// TestOnLoginLinkReportsRosterLargerThanOneFrame links up with more
// players in the world than one PlayerInGame frame holds: the roster goes
// over several frames, the login server lists every account online, and
// the link stays up.
func TestOnLoginLinkReportsRosterLargerThanOneFrame(t *testing.T) {
	addr, servers := startLoginServer(t)
	key := []byte{0x11, 0x22, 0x33, 0x44}
	servers.Register(1, key)
	answers := make(chan string, 1)
	l, err := network.DialLoginLink(context.Background(), addr,
		network.LoginServerAuth{ServerID: 1, HexID: key, HostName: "*", Port: 7777, MaxPlayers: 5000},
		network.LoginLinkHandlers{PlayerAuthResponse: func(account string, _ bool) { answers <- account }}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()

	// 14-character names, the longest an account has: 30 bytes each, so a
	// frame holds 2,184 of them.
	const online = 2300
	state := world.New()
	for i := range online {
		state.AddPlayer(&rosterPlayerStub{id: int32(i + 1), account: fmt.Sprintf("roster%08d", i)})
	}
	onLoginLink(provideLoginLinkState(), network.NewLoginStatus(false, 5000, func(link.ServerStatus) {}),
		state, filepath.Join(t.TempDir(), "hexid.txt"), key, zerolog.Nop())(l)

	syncLink(t, l, answers)
	select {
	case <-l.Done():
		t.Fatal("link ended after the roster send")
	default:
	}
	if got := servers.OnlineAccountCount(1); got != online {
		t.Fatalf("login server lists %d accounts online, want %d", got, online)
	}
	for i := range online {
		if id, ok := servers.AccountServerID(fmt.Sprintf("roster%08d", i)); !ok || id != 1 {
			t.Fatalf("account %d online on server %d, %v; want server 1", i, id, ok)
		}
	}
}
