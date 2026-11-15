package loginserver

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// serveGameServerLinkFor starts a game-server link over the client link's
// own server registry and session store, so a fake game server checks the
// sessions that client logins issue.
func serveGameServerLinkFor(t *testing.T, servers *manager.ServerRegistry, sessions *manager.SessionStore) string {
	t.Helper()
	namesPath := filepath.Join(t.TempDir(), "serverNames.xml")
	if err := os.WriteFile(namesPath, []byte(`<?xml version='1.0'?><list><server id="1" name="Bartz" /></list>`), 0o644); err != nil {
		t.Fatalf("write serverNames.xml: %v", err)
	}
	names, err := manager.LoadServerNames(namesPath)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}
	keys, err := manager.NewRSAKeyPool()
	if err != nil {
		t.Fatalf("NewRSAKeyPool: %v", err)
	}
	gsLink := NewGameServerLink(servers, names, keys, sessions, manager.NewIPBanList(zerolog.Nop()), nil, nil, false, nil, NewLinkRoster(), zerolog.Nop())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go gsLink.Serve(ctx, ln)
	return ln.Addr().String()
}

// TestPlayKeysComeOnlyFromAnAdmittedServerLogin logs in an access-level-0
// account and, without any server login, has the game server present the
// login pair with zero play keys for a GM-only server: the login server
// refuses, since the play keys it issued at login are not zero. A GM
// account's admitted server login hands over play keys the game server's
// check then accepts.
func TestPlayKeysComeOnlyFromAnAdmittedServerLogin(t *testing.T) {
	accounts := newFakeAccountStore(
		model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1),
		model.NewAccount("gm", mustHashPassword(t, "s3cret"), 1, 1),
	)
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	servers.Register(1, testHexID)
	gs := dialGameServer(t, serveGameServerLinkFor(t, servers, sessions))
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("game server registration failed")
	}
	gmOnly := link.ServerTypeGMOnly
	servers.ApplyStatus(1, link.ServerStatus{Status: &gmOnly})

	c := dialLoginClient(t, addr)
	loginKey1, loginKey2 := c.login(l, "player1", "s3cret")
	gs.sendPlayerAuthRequest("player1", link.SessionKey{LoginKey1: loginKey1, LoginKey2: loginKey2})
	if account, ok := gs.readPlayerAuthResponse(); account != "player1" || ok {
		t.Fatalf("PlayerAuthResponse without a server login = %q, %v, want player1, false", account, ok)
	}

	gm := dialLoginClient(t, addr)
	loginKey1, loginKey2 = gm.login(l, "gm", "s3cret")
	gm.send(encodeRequestServerLogin(loginKey1, loginKey2, 1))
	reply := gm.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("gm server login opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	playKey1, playKey2 := r.ReadInt32(), r.ReadInt32()
	gs.sendPlayerAuthRequest("gm", link.SessionKey{LoginKey1: loginKey1, LoginKey2: loginKey2, PlayKey1: playKey1, PlayKey2: playKey2})
	if account, ok := gs.readPlayerAuthResponse(); account != "gm" || !ok {
		t.Fatalf("PlayerAuthResponse after an admitted server login = %q, %v, want gm, true", account, ok)
	}
}
