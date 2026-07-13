package loginserver

import (
	"context"
	"crypto/rsa"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// --- fake account store: no DB needed to exercise ClientLink's own logic ---

type fakeAccountStore struct {
	mu       sync.Mutex
	accounts map[string]model.Account
}

func newFakeAccountStore(accs ...model.Account) *fakeAccountStore {
	m := make(map[string]model.Account, len(accs))
	for _, a := range accs {
		m[a.Login] = a
	}
	return &fakeAccountStore{accounts: m}
}

func (s *fakeAccountStore) Account(_ context.Context, login string) (model.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[login]
	if !ok {
		return model.Account{}, loginsql.ErrAccountNotFound
	}
	return a, nil
}

func (s *fakeAccountStore) CreateAccount(_ context.Context, login, hashedPassword string, _ time.Time) (model.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := model.NewAccount(login, hashedPassword, 0, 1)
	s.accounts[login] = a
	return a, nil
}

func (s *fakeAccountStore) SetLastServer(_ context.Context, login string, serverID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.accounts[login]
	a.LastServer = serverID
	s.accounts[login] = a
	return nil
}

func (s *fakeAccountStore) get(login string) (model.Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[login]
	return a, ok
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hashed, err := model.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return hashed
}

// --- fake login client, driving the wire protocol from the other side ---
//
// ClientLink.newSessionKey is overridden to a fixed key for every test
// connection, so the fake client never needs to reverse the static-key/XOR
// scheme that protects the real Init packet (that decoding is the real L2
// client's job, deliberately not built here — see logincrypt.LoginCrypt's
// doc comment). It only discards the Init frame and talks dynamic-key
// Blowfish+checksum from then on, exactly like a real client does for every
// packet after Init.

var testSessionKey = []byte("0123456789abcdef")

type fakeLoginClient struct {
	t      *testing.T
	conn   net.Conn
	cipher *commoncrypt.BlowfishCipher
}

func dialLoginClient(t *testing.T, addr string) *fakeLoginClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := wire.ReadFrame(conn); err != nil {
		t.Fatalf("read Init frame: %v", err)
	}

	cipher, err := commoncrypt.NewBlowfishCipher(testSessionKey)
	if err != nil {
		t.Fatalf("NewBlowfishCipher: %v", err)
	}
	return &fakeLoginClient{t: t, conn: conn, cipher: cipher}
}

func (f *fakeLoginClient) send(payload []byte) {
	f.t.Helper()
	buf := make([]byte, commoncrypt.PaddedSize(len(payload)+4))
	copy(buf, payload)
	commoncrypt.AppendChecksum(buf)
	commoncrypt.EncryptBlocks(f.cipher, buf)
	if err := wire.WriteFrame(f.conn, buf); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

func (f *fakeLoginClient) read() []byte {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	commoncrypt.DecryptBlocks(f.cipher, payload)
	if !commoncrypt.VerifyChecksum(payload) {
		f.t.Fatalf("bad checksum on inbound frame")
	}
	return payload
}

func (f *fakeLoginClient) expectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := f.conn.Read(buf); n != 0 || err == nil {
		f.t.Fatalf("expected connection to close, got n=%d err=%v", n, err)
	}
}

func encodeAuthGameGuard(sessionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthGameGuard)
	w.WriteInt32(sessionID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

// encodeRequestAuthLogin builds a raw RequestAuthLogin payload: the
// credential block RSA-encrypted (no padding scheme) with pub, matching
// DecodeRequestAuthLogin's fixed username/password offsets.
func encodeRequestAuthLogin(pub *rsa.PublicKey, username, password string) []byte {
	var block [128]byte
	copy(block[0x5e:0x5e+14], username)
	copy(block[0x6c:0x6c+16], password)
	ciphertext := commoncrypt.EncryptDynamicKey(pub, block[:])

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAuthLogin)
	w.WriteBytes(ciphertext)
	return w.Bytes()
}

func encodeRequestServerList(key1, key2 int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestServerList)
	w.WriteInt32(key1)
	w.WriteInt32(key2)
	return w.Bytes()
}

func encodeRequestServerLogin(key1, key2 int32, serverID byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestServerLogin)
	w.WriteInt32(key1)
	w.WriteInt32(key2)
	w.WriteUint8(serverID)
	return w.Bytes()
}

// --- test server setup ---

func newTestClientLink(t *testing.T, accounts *fakeAccountStore, autoCreate bool) (addr string, l *ClientLink, servers *manager.ServerRegistry, sessions *manager.SessionStore, bans *manager.IPBanList) {
	t.Helper()

	keyPair, err := commoncrypt.NewLoginKeyPair()
	if err != nil {
		t.Fatalf("NewLoginKeyPair: %v", err)
	}

	servers = manager.NewServerRegistry()
	sessions = manager.NewSessionStore()
	bans = manager.NewIPBanList(zerolog.Nop())

	l = &ClientLink{
		accounts:           accounts,
		servers:            servers,
		sessions:           sessions,
		bans:               bans,
		autoCreateAccounts: autoCreate,
		log:                zerolog.Nop(),
		newKeyPair:         func() *commoncrypt.LoginKeyPair { return keyPair },
		newSessionKey:      func() ([]byte, error) { return testSessionKey, nil },
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go l.Serve(ctx, ln)

	return ln.Addr().String(), l, servers, sessions, bans
}

// loginKeyPair exposes the test ClientLink's fixed RSA key pair for
// building RequestAuthLogin payloads.
func (l *ClientLink) loginKeyPair() *commoncrypt.LoginKeyPair {
	return l.newKeyPair()
}

func TestClientLinkSendsInitOnConnect(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	dialLoginClient(t, addr) // dial fails the test itself if Init never arrives
}

func TestClientLinkAuthGameGuardRepliesGGAuth(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	c := dialLoginClient(t, addr)

	c.send(encodeAuthGameGuard(1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeGGAuth {
		t.Fatalf("opcode = %#x, want GGAuth (%#x)", reply[0], serverpackets.OpcodeGGAuth)
	}
}

func TestClientLinkLoginSuccess(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginOk {
		t.Fatalf("opcode = %#x, want LoginOk (%#x)", reply[0], serverpackets.OpcodeLoginOk)
	}

	key, ok := sessions.Get("player1")
	if !ok {
		t.Fatal("expected a session to be stored for player1")
	}
	r := wire.NewReader(reply[1:])
	wantKey1, wantKey2 := r.ReadInt32(), r.ReadInt32()
	if key.LoginKey1 != wantKey1 || key.LoginKey2 != wantKey2 {
		t.Fatalf("stored session key = %+v, want login key halves %d/%d", key, wantKey1, wantKey2)
	}
}

func TestClientLinkLoginWrongPassword(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	c.expectClosed()
}

func TestClientLinkLoginUnknownAccountAutoCreateOff(t *testing.T) {
	accounts := newFakeAccountStore()
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "newplayer", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	c.expectClosed()

	if _, ok := accounts.get("newplayer"); ok {
		t.Fatal("account should not have been created")
	}
}

func TestClientLinkLoginUnknownAccountAutoCreateOn(t *testing.T) {
	accounts := newFakeAccountStore()
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, true)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "newplayer", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginOk {
		t.Fatalf("opcode = %#x, want LoginOk (%#x)", reply[0], serverpackets.OpcodeLoginOk)
	}

	acc, ok := accounts.get("newplayer")
	if !ok {
		t.Fatal("expected account to be auto-created")
	}
	if bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte("s3cret")) != nil {
		t.Fatal("auto-created account password does not match")
	}
	if _, ok := sessions.Get("newplayer"); !ok {
		t.Fatal("expected a session to be stored for the auto-created account")
	}
}

func TestClientLinkLoginBannedAccountRejected(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("banned", mustHashPassword(t, "s3cret"), -1, 1))
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "banned", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAccountKicked {
		t.Fatalf("opcode = %#x, want AccountKicked (%#x)", reply[0], serverpackets.OpcodeAccountKicked)
	}
	c.expectClosed()
}

func TestClientLinkLoginDuplicateSessionRejected(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)
	sessions.Put("player1", link.SessionKey{LoginKey1: 1, LoginKey2: 2})

	c := dialLoginClient(t, addr)
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	c.expectClosed()
}

// login drives a fake client through a successful RequestAuthLogin and
// returns the two session-key halves LoginOk carried.
func (f *fakeLoginClient) login(l *ClientLink, username, password string) (key1, key2 int32) {
	f.t.Helper()
	f.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, username, password))
	reply := f.read()
	if reply[0] != serverpackets.OpcodeLoginOk {
		f.t.Fatalf("login opcode = %#x, want LoginOk (%#x)", reply[0], serverpackets.OpcodeLoginOk)
	}
	r := wire.NewReader(reply[1:])
	return r.ReadInt32(), r.ReadInt32()
}

func TestClientLinkServerList(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 2))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", 7777, 100)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerList(key1, key2))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeServerList {
		t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
	}
	if count := reply[1]; count != 1 {
		t.Fatalf("server count = %d, want 1", count)
	}
	if last := reply[2]; last != 2 {
		t.Fatalf("last server = %d, want 2 (account.LastServer)", last)
	}
	if id := reply[3]; id != 7 {
		t.Fatalf("server id = %d, want 7", id)
	}
}

func TestClientLinkServerEntriesUseAdvertisedStatusForOnlineByte(t *testing.T) {
	servers := manager.NewServerRegistry()
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", 7777, 100)

	l := &ClientLink{servers: servers}
	entries := l.serverEntries()
	if len(entries) != 1 {
		t.Fatalf("serverEntries length = %d, want 1", len(entries))
	}
	if entries[0].Online {
		t.Fatal("Online = true while status is Down")
	}

	auto := link.ServerTypeAuto
	servers.ApplyStatus(7, link.ServerStatus{Status: &auto})

	entries = l.serverEntries()
	if len(entries) != 1 || !entries[0].Online {
		t.Fatalf("serverEntries = %+v, want one online entry after Status=Auto", entries)
	}
}

func TestClientLinkPlayLoginSuccess(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", 7777, 100)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	playKey1, playKey2 := r.ReadInt32(), r.ReadInt32()

	full, ok := sessions.Get("player1")
	if !ok {
		t.Fatal("expected session to remain stored after PlayOk")
	}
	want := link.SessionKey{LoginKey1: key1, LoginKey2: key2, PlayKey1: playKey1, PlayKey2: playKey2}
	if full != want {
		t.Fatalf("stored session = %+v, want %+v", full, want)
	}

	acc, _ := accounts.get("player1")
	if acc.LastServer != 7 {
		t.Fatalf("account.LastServer = %d, want 7", acc.LastServer)
	}
}

func TestClientLinkDisconnectBeforeGameServerJoinReleasesSession(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
	if _, ok := sessions.Get("player1"); !ok {
		t.Fatal("expected session to be stored after LoginOk")
	}

	if err := c.conn.Close(); err != nil {
		t.Fatalf("close login client: %v", err)
	}
	waitSessionMissing(t, sessions, "player1")

	c = dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
}

func TestClientLinkDisconnectAfterPlayOkKeepsSessionForGameServer(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
	servers.Register(7, []byte{0x01})
	servers.MarkOnline(7, "127.0.0.1", 7777, 100)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")
	c.send(encodeRequestServerLogin(key1, key2, 7))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayOk {
		t.Fatalf("opcode = %#x, want PlayOk (%#x)", reply[0], serverpackets.OpcodePlayOk)
	}
	r := wire.NewReader(reply[1:])
	want := link.SessionKey{
		LoginKey1: key1,
		LoginKey2: key2,
		PlayKey1:  r.ReadInt32(),
		PlayKey2:  r.ReadInt32(),
	}

	if err := c.conn.Close(); err != nil {
		t.Fatalf("close login client: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	got, ok := sessions.Get("player1")
	if !ok {
		t.Fatal("expected session to remain after PlayOk")
	}
	if got != want {
		t.Fatalf("stored session = %+v, want %+v", got, want)
	}
}

func TestClientLinkPlayLoginUnknownServerFails(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)

	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")

	c.send(encodeRequestServerLogin(key1, key2, 9))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePlayFail {
		t.Fatalf("opcode = %#x, want PlayFail (%#x)", reply[0], serverpackets.OpcodePlayFail)
	}
}

func TestClientLinkBannedIPRejected(t *testing.T) {
	addr, _, _, _, bans := newTestClientLink(t, newFakeAccountStore(), false)
	bans.Ban(net.ParseIP("127.0.0.1"), 0)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := conn.Read(buf); n != 0 || err == nil {
		t.Fatalf("expected connection to close without sending Init, got n=%d err=%v", n, err)
	}
}

func TestClientLinkOpcodeBeforeAuthCloses(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestServerList(1, 2))
	c.expectClosed()
}

func waitSessionMissing(t *testing.T, sessions *manager.SessionStore, account string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := sessions.Get(account); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session for %q remained stored", account)
}
