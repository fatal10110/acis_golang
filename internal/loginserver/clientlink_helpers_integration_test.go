package loginserver

import (
	"context"
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"net"
	"os"
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
//
// docs/agents/test-strategy.md: loginsql.AccountStore already satisfies the same
// Account/CreateAccount/SetLastServer method set and is exercised for real
// in internal/loginserver/data/sql/account_integration_test.go. Kept as a
// fake here because these tests are about ClientLink's protocol/session
// logic (auth flow, rejections, session lifecycle), not SQL persistence —
// routing every one of them through a MariaDB container would be
// disproportionate to what they verify.

type fakeAccountStore struct {
	mu            sync.Mutex
	accounts      map[string]model.Account
	lastActive    map[string]time.Time
	lastActiveErr error
}

func newFakeAccountStore(accs ...model.Account) *fakeAccountStore {
	m := make(map[string]model.Account, len(accs))
	for _, a := range accs {
		m[a.Login] = a
	}
	return &fakeAccountStore{accounts: m, lastActive: make(map[string]time.Time)}
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

func (s *fakeAccountStore) SetLastActive(_ context.Context, login string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastActiveErr != nil {
		return s.lastActiveErr
	}
	s.lastActive[login] = at
	return nil
}

func (s *fakeAccountStore) get(login string) (model.Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.accounts[login]
	return a, ok
}

func (s *fakeAccountStore) getLastActive(login string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.lastActive[login]
	return at, ok
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	// MinCost keeps fixtures cheap; verification reads the cost from the hash.
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(hashed)
}

// minCostHashPassword replaces model.HashPassword on test ClientLinks. The
// production cost takes most of a second per hash under -race, which on a
// loaded host pushes the auto-create login past the fake client's read
// deadline; MinCost keeps the same bcrypt format and verification path.
func minCostHashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	return string(hashed), err
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
//
// docs/agents/test-strategy.md: this is a test-harness actor, not a stand-in for a
// production interface — it plays the real L2 client's side of the wire so
// ClientLink's own real auth/session code runs end-to-end over a real TCP
// socket. There is no production type to substitute; keep as-is.

var testSessionKey = []byte("0123456789abcdef")

// testInitSessionID is the deterministic Init session id every test
// ClientLink hands out, so tests can assert on session-id echoes.
const testInitSessionID int32 = 0x5a5a5a5a

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

// expectClosed requires the server to have closed the connection with no
// further bytes: a read that times out on a still-open, silent socket fails.
func (f *fakeLoginClient) expectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := f.conn.Read(buf); n != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
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

// gameGuard completes the AuthGameGuard exchange with the Init session id,
// as every real client does after receiving Init.
func (f *fakeLoginClient) gameGuard() {
	f.t.Helper()
	f.send(encodeAuthGameGuard(testInitSessionID))
	reply := f.read()
	if reply[0] != serverpackets.OpcodeGGAuth {
		f.t.Fatalf("opcode = %#x, want GGAuth (%#x)", reply[0], serverpackets.OpcodeGGAuth)
	}
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

func newTestClientLink(t *testing.T, accounts *fakeAccountStore, autoCreate bool, opts ...func(*ClientLink)) (addr string, l *ClientLink, servers *manager.ServerRegistry, sessions *manager.SessionStore, bans *manager.IPBanList) {
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
		loginTryBeforeBan:  DefaultLoginTryBeforeBan,
		loginBlockAfterBan: DefaultLoginBlockAfterBan,
		log:                zerolog.Nop(),
		newKeyPair:         func() *commoncrypt.LoginKeyPair { return keyPair },
		newSessionKey:      func() ([]byte, error) { return testSessionKey, nil },
		newSessionID:       func() int32 { return testInitSessionID },
		hashPassword:       minCostHashPassword,
	}
	for _, opt := range opts {
		opt(l)
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

func loginFailReason(t *testing.T, reply []byte) serverpackets.LoginFailReason {
	t.Helper()
	return serverpackets.LoginFailReason(binary.LittleEndian.Uint32(reply[1:5]))
}

// login drives a fake client through a successful RequestAuthLogin and
// returns the two session-key halves LoginOk carried.
func (f *fakeLoginClient) login(l *ClientLink, username, password string) (key1, key2 int32) {
	f.t.Helper()
	f.gameGuard()
	f.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, username, password))
	reply := f.read()
	if reply[0] != serverpackets.OpcodeLoginOk {
		f.t.Fatalf("login opcode = %#x, want LoginOk (%#x)", reply[0], serverpackets.OpcodeLoginOk)
	}
	r := wire.NewReader(reply[1:])
	return r.ReadInt32(), r.ReadInt32()
}

// markOnlineAuto links server id and raises its reported status to Auto, as
// a game server does with its first ServerStatus packet; until then the
// server counts as down and admits nobody.
func markOnlineAuto(t *testing.T, servers *manager.ServerRegistry, id int) {
	t.Helper()
	servers.Register(id, []byte{0x01})
	servers.MarkOnline(id, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)
	auto := link.ServerTypeAuto
	if _, ok := servers.ApplyStatus(id, link.ServerStatus{Status: &auto}); !ok {
		t.Fatalf("ApplyStatus(%d) = false", id)
	}
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
