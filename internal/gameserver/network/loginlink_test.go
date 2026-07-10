package network

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
)

// newTestLoginServer starts a real login-server-side GS-LS link acceptor on
// an ephemeral port, giving loginlink_test.go's DialLoginLink calls a live
// server to complete the full handshake against, not just hand-rolled wire
// bytes.
func newTestLoginServer(t *testing.T, allowNewServers bool) (addr string, servers *manager.ServerRegistry, sessions *manager.SessionStore) {
	t.Helper()

	dir := t.TempDir()
	namesPath := filepath.Join(dir, "serverNames.xml")
	if err := os.WriteFile(namesPath, []byte(`<?xml version='1.0'?><list>
		<server id="1" name="Bartz" />
		<server id="2" name="Sieghardt" />
	</list>`), 0o644); err != nil {
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

	servers = manager.NewServerRegistry()
	sessions = manager.NewSessionStore()
	bans := manager.NewIPBanList(zerolog.Nop())

	gsLink := loginserver.NewGameServerLink(servers, names, keys, sessions, bans, nil, nil, allowNewServers, zerolog.Nop())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go gsLink.Serve(ctx, ln)

	return ln.Addr().String(), servers, sessions
}

func TestGenerateDynamicKeyRedrawsOnLeadingZeroByte(t *testing.T) {
	badDraw := append([]byte{0x00}, bytes.Repeat([]byte{0xaa}, dynamicKeySize-1)...)
	goodDraw := bytes.Repeat([]byte{0xbb}, dynamicKeySize)
	src := io.MultiReader(bytes.NewReader(badDraw), bytes.NewReader(goodDraw))

	key, err := generateDynamicKey(src)
	if err != nil {
		t.Fatalf("generateDynamicKey: %v", err)
	}
	if !bytes.Equal(key, goodDraw) {
		t.Fatalf("generateDynamicKey() = %x, want the redrawn key %x (a leading zero byte draw must be rejected)", key, goodDraw)
	}
}

func TestGenerateDynamicKeyAcceptsNonZeroLeadingDraw(t *testing.T) {
	goodDraw := bytes.Repeat([]byte{0xcc}, dynamicKeySize)

	key, err := generateDynamicKey(bytes.NewReader(goodDraw))
	if err != nil {
		t.Fatalf("generateDynamicKey: %v", err)
	}
	if !bytes.Equal(key, goodDraw) {
		t.Fatalf("generateDynamicKey() = %x, want %x", key, goodDraw)
	}
}

var testHexID = []byte{0x01, 0x02, 0x03, 0x04}

func TestDialLoginLinkRegistersAndAuths(t *testing.T) {
	// Fast tests never exercise a fresh registration, so the DB-backed
	// stores stay nil and unused: pre-register the server id as if from a
	// prior boot's DB load, same as the login server's own acceptor tests.
	addr, servers, _ := newTestLoginServer(t, true)
	servers.Register(1, testHexID)

	auth := LoginServerAuth{
		ServerID:          1,
		AcceptAlternateID: false,
		HexID:             testHexID,
		HostName:          "*",
		Port:              7777,
		MaxPlayers:        300,
	}

	l, err := DialLoginLink(context.Background(), addr, auth, LoginLinkHandlers{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()

	if l.ServerID != 1 || l.ServerName != "Bartz" {
		t.Fatalf("ServerID/ServerName = %d/%q, want 1/Bartz", l.ServerID, l.ServerName)
	}

	entry, exists := servers.Get(1)
	if !exists || !entry.Authed || entry.Port != 7777 || entry.MaxPlayers != 300 {
		t.Fatalf("registry entry after auth = %+v", entry)
	}
}

func TestDialLoginLinkWrongHexIDRejected(t *testing.T) {
	addr, servers, _ := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	auth := LoginServerAuth{ServerID: 1, HexID: []byte{0xff, 0xff}, HostName: "*", Port: 7777, MaxPlayers: 300}

	_, err := DialLoginLink(context.Background(), addr, auth, LoginLinkHandlers{}, zerolog.Nop())
	if err == nil {
		t.Fatal("DialLoginLink: want error for mismatched hex id, got nil")
	}
	if !strings.Contains(err.Error(), link.ReasonWrongHexID.String()) {
		t.Fatalf("DialLoginLink error = %v, want it to mention %q", err, link.ReasonWrongHexID)
	}
}

func TestDialLoginLinkAlreadyLoggedInRejected(t *testing.T) {
	addr, servers, _ := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}

	first, err := DialLoginLink(context.Background(), addr, auth, LoginLinkHandlers{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("first DialLoginLink: %v", err)
	}
	defer first.Close()

	_, err = DialLoginLink(context.Background(), addr, auth, LoginLinkHandlers{}, zerolog.Nop())
	if err == nil {
		t.Fatal("second DialLoginLink: want error for already-logged-in server, got nil")
	}
	if !strings.Contains(err.Error(), link.ReasonAlreadyLoggedIn.String()) {
		t.Fatalf("second DialLoginLink error = %v, want it to mention %q", err, link.ReasonAlreadyLoggedIn)
	}
}

func TestDialLoginLinkPlayerAuthRequestRoundTrip(t *testing.T) {
	addr, servers, sessions := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	responses := make(chan struct {
		account string
		ok      bool
	}, 1)
	handlers := LoginLinkHandlers{
		PlayerAuthResponse: func(account string, ok bool) {
			responses <- struct {
				account string
				ok      bool
			}{account, ok}
		},
	}

	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	l, err := DialLoginLink(context.Background(), addr, auth, handlers, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()

	key := link.SessionKey{PlayKey1: 1, PlayKey2: 2, LoginKey1: 3, LoginKey2: 4}
	sessions.Put("acc1", key)

	if err := l.SendPlayerAuthRequest(link.PlayerAuthRequest{
		Account:    "acc1",
		SessionKey: key,
	}); err != nil {
		t.Fatalf("SendPlayerAuthRequest: %v", err)
	}

	select {
	case resp := <-responses:
		if resp.account != "acc1" || !resp.ok {
			t.Fatalf("PlayerAuthResponse = %+v, want acc1/true", resp)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for PlayerAuthResponse")
	}
}

func TestDialLoginLinkPlayerInGameAndStatus(t *testing.T) {
	addr, servers, _ := newTestLoginServer(t, true)
	servers.Register(1, testHexID)

	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	l, err := DialLoginLink(context.Background(), addr, auth, LoginLinkHandlers{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()

	maxPlayers := int32(42)
	if err := l.SendServerStatus(link.ServerStatus{MaxPlayers: &maxPlayers}); err != nil {
		t.Fatalf("SendServerStatus: %v", err)
	}
	if err := l.SendPlayerInGame([]string{"acc1", "acc2"}); err != nil {
		t.Fatalf("SendPlayerInGame: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		entry, _ := servers.Get(1)
		if entry.MaxPlayers == 42 && servers.OnlineAccountCount(1) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("registry did not observe status/player-in-game updates: entry=%+v online=%d", entry, servers.OnlineAccountCount(1))
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := l.SendPlayerLogout("acc1"); err != nil {
		t.Fatalf("SendPlayerLogout: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for servers.OnlineAccountCount(1) != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("OnlineAccountCount() = %d, want 1", servers.OnlineAccountCount(1))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fakeLoginServer is a bare-bones GS-LS link peer for handshake edge cases
// the real login server's acceptor never produces on its own (a revision
// mismatch, a malformed frame).
type fakeLoginServer struct {
	t    *testing.T
	ln   net.Listener
	priv *rsa.PrivateKey
}

func newFakeLoginServer(t *testing.T) *fakeLoginServer {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return &fakeLoginServer{t: t, ln: ln, priv: priv}
}

func (f *fakeLoginServer) addr() string { return f.ln.Addr().String() }

// acceptAndSendInitLS accepts one connection and sends a raw (unencrypted-
// beyond-bootstrap) InitLS payload built from rawInitLS, letting a test
// craft a protocol revision the real login server would never send.
func (f *fakeLoginServer) acceptAndSendInitLS(rawInitLS []byte) {
	go func() {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		bootstrap := crypt.NewLinkCrypt()
		wire.WriteFrame(conn, bootstrap.Encrypt(rawInitLS))
	}()
}

func TestDialLoginLinkRevisionMismatch(t *testing.T) {
	fake := newFakeLoginServer(t)

	var badInitLS []byte
	badInitLS = append(badInitLS, link.OpcodeInitLS)
	badInitLS = binary.LittleEndian.AppendUint32(badInitLS, 0xdead)
	modulus := fake.priv.PublicKey.N.Bytes()
	badInitLS = binary.LittleEndian.AppendUint32(badInitLS, uint32(len(modulus)))
	badInitLS = append(badInitLS, modulus...)

	fake.acceptAndSendInitLS(badInitLS)

	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	_, err := DialLoginLink(context.Background(), fake.addr(), auth, LoginLinkHandlers{}, zerolog.Nop())
	if err == nil {
		t.Fatal("DialLoginLink: want error for protocol revision mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "revision") {
		t.Fatalf("DialLoginLink error = %v, want it to mention revision", err)
	}
}
