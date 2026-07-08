package loginserver

import (
	"context"
	"crypto/rsa"
	"encoding/binary"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/sirupsen/logrus"
)

// --- fake game-server client, driving the wire protocol from the other side ---

type fakeGameServer struct {
	t     *testing.T
	conn  net.Conn
	crypt *crypt.LinkCrypt
	pub   *rsa.PublicKey
}

func dialGameServer(t *testing.T, addr string) *fakeGameServer {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return &fakeGameServer{t: t, conn: conn, crypt: crypt.NewLinkCrypt()}
}

func (f *fakeGameServer) readFrame() []byte {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if err := f.crypt.Decrypt(payload); err != nil {
		f.t.Fatalf("Decrypt: %v", err)
	}
	return payload
}

func (f *fakeGameServer) sendFrame(payload []byte) {
	f.t.Helper()
	if err := wire.WriteFrame(f.conn, f.crypt.Encrypt(payload)); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

// expectClosed asserts the login server closed the connection without
// sending anything further.
func (f *fakeGameServer) expectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := f.conn.Read(buf); n != 0 || err == nil {
		f.t.Fatalf("expected connection to close, got n=%d err=%v", n, err)
	}
}

// frameReader decodes a decrypted link payload positionally, starting
// after its opcode byte. LinkCrypt pads and checksums every encrypted
// frame, so a decrypted payload carries trailing padding/checksum bytes
// after its real content — this must read fields from the front and stop,
// exactly like the production decoders do, rather than ever indexing from
// the end of the slice.
type frameReader struct {
	buf []byte
	pos int
}

func newFrameReader(buf []byte) *frameReader { return &frameReader{buf: buf, pos: 1} }

func (r *frameReader) readByte() byte {
	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *frameReader) readUint16() uint16 {
	v := binary.LittleEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v
}

func (r *frameReader) readInt32() int32 {
	v := int32(binary.LittleEndian.Uint32(r.buf[r.pos:]))
	r.pos += 4
	return v
}

func (r *frameReader) readBytes(n int) []byte {
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *frameReader) readString() string {
	start := r.pos
	end := start
	for end+1 < len(r.buf) && (r.buf[end] != 0 || r.buf[end+1] != 0) {
		end += 2
	}
	units := make([]uint16, (end-start)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(r.buf[start+i*2:])
	}
	r.pos = end + 2
	return string(utf16.Decode(units))
}

// readInitLS reads and parses the handshake's opening InitLS packet,
// caching the login server's public key for later use.
func (f *fakeGameServer) readInitLS() {
	f.t.Helper()
	payload := f.readFrame()
	if payload[0] != link.OpcodeInitLS {
		f.t.Fatalf("first packet opcode = %#x, want InitLS", payload[0])
	}
	r := newFrameReader(payload)
	_ = r.readInt32() // protocol revision
	size := int(r.readInt32())
	keyBytes := r.readBytes(size)
	f.pub = &rsa.PublicKey{N: new(big.Int).SetBytes(keyBytes), E: 65537}
}

// sendBlowFishKey RSA-encrypts key with the login server's public key and
// sends it, then switches this client's own crypt to key.
func (f *fakeGameServer) sendBlowFishKey(key []byte) {
	f.t.Helper()
	m := new(big.Int).SetBytes(key)
	c := new(big.Int).Exp(m, big.NewInt(int64(f.pub.E)), f.pub.N)
	ciphertext := c.Bytes()

	payload := []byte{link.OpcodeBlowFishKey}
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(ciphertext)))
	payload = append(payload, ciphertext...)
	f.sendFrame(payload)

	if err := f.crypt.SetKey(key); err != nil {
		f.t.Fatalf("SetKey: %v", err)
	}
}

func writeUTF16String(buf []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		buf = binary.LittleEndian.AppendUint16(buf, u)
	}
	return binary.LittleEndian.AppendUint16(buf, 0)
}

// sendGameServerAuth sends a registration/re-authentication request.
func (f *fakeGameServer) sendGameServerAuth(id byte, acceptAlternate, hostReserved bool, host string, port uint16, maxPlayers int32, hexID []byte) {
	f.t.Helper()
	payload := []byte{link.OpcodeGameServerAuth, id, boolByte(acceptAlternate), boolByte(hostReserved)}
	payload = writeUTF16String(payload, host)
	payload = binary.LittleEndian.AppendUint16(payload, port)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(maxPlayers))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(hexID)))
	payload = append(payload, hexID...)
	f.sendFrame(payload)
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// readAuthResponse reads either an AuthResponse (ok=true) or a
// LoginServerFail (ok=false).
func (f *fakeGameServer) readAuthResult() (ok bool, serverID byte, name string, failReason byte) {
	f.t.Helper()
	payload := f.readFrame()
	r := newFrameReader(payload)
	switch payload[0] {
	case link.OpcodeAuthResponse:
		id := r.readByte()
		s := r.readString()
		return true, id, s, 0
	case link.OpcodeLoginServerFail:
		return false, 0, "", r.readByte()
	default:
		f.t.Fatalf("unexpected opcode %#x, want AuthResponse or LoginServerFail", payload[0])
		return false, 0, "", 0
	}
}

func (f *fakeGameServer) sendServerStatus(attrs map[int32]int32) {
	f.t.Helper()
	payload := []byte{link.OpcodeServerStatus}
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(attrs)))
	for attr, value := range attrs {
		payload = binary.LittleEndian.AppendUint32(payload, uint32(attr))
		payload = binary.LittleEndian.AppendUint32(payload, uint32(value))
	}
	f.sendFrame(payload)
}

func (f *fakeGameServer) sendPlayerInGame(accounts ...string) {
	f.t.Helper()
	payload := []byte{link.OpcodePlayerInGame}
	payload = binary.LittleEndian.AppendUint16(payload, uint16(len(accounts)))
	for _, a := range accounts {
		payload = writeUTF16String(payload, a)
	}
	f.sendFrame(payload)
}

func (f *fakeGameServer) sendPlayerLogout(account string) {
	f.t.Helper()
	payload := []byte{link.OpcodePlayerLogout}
	payload = writeUTF16String(payload, account)
	f.sendFrame(payload)
}

func (f *fakeGameServer) sendChangeAccessLevel(level int32, account string) {
	f.t.Helper()
	payload := []byte{link.OpcodeChangeAccessLevel}
	payload = binary.LittleEndian.AppendUint32(payload, uint32(level))
	payload = writeUTF16String(payload, account)
	f.sendFrame(payload)
}

func (f *fakeGameServer) sendPlayerAuthRequest(account string, key link.SessionKey) {
	f.t.Helper()
	payload := []byte{link.OpcodePlayerAuthRequest}
	payload = writeUTF16String(payload, account)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(key.PlayKey1))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(key.PlayKey2))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(key.LoginKey1))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(key.LoginKey2))
	f.sendFrame(payload)
}

func (f *fakeGameServer) readPlayerAuthResponse() (account string, ok bool) {
	f.t.Helper()
	payload := f.readFrame()
	if payload[0] != link.OpcodePlayerAuthResponse {
		f.t.Fatalf("opcode = %#x, want PlayerAuthResponse", payload[0])
	}
	r := newFrameReader(payload)
	account = r.readString()
	return account, r.readByte() != 0
}

// --- test server setup ---

func newTestLink(t *testing.T, allowNewServers bool) (addr string, l *GameServerLink, servers *manager.ServerRegistry, sessions *manager.SessionStore, bans *manager.IPBanList) {
	t.Helper()
	// Fast tests never exercise a fresh registration or ChangeAccessLevel,
	// so the DB-backed stores are never dereferenced; nil is safe here and
	// keeps this test hermetic. The DB-touching paths are covered by the
	// integration test, via newTestLinkCommon with real stores.
	return newTestLinkCommon(t, allowNewServers, nil, nil)
}

func newTestLinkCommon(t *testing.T, allowNewServers bool, accounts *sql.AccountStore, registrations *sql.GameServerStore) (addr string, l *GameServerLink, servers *manager.ServerRegistry, sessions *manager.SessionStore, bans *manager.IPBanList) {
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
	bans = manager.NewIPBanList(logrus.StandardLogger())

	l = NewGameServerLink(servers, names, keys, sessions, bans, accounts, registrations, allowNewServers, logrus.StandardLogger())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go l.Serve(ctx, ln)

	return ln.Addr().String(), l, servers, sessions, bans
}

var testHexID = []byte{0x01, 0x02, 0x03, 0x04}

// handshake drives InitLS + BlowFishKey, leaving the fake client ready to
// send GameServerAuth.
func (f *fakeGameServer) handshake() {
	f.t.Helper()
	f.readInitLS()
	f.sendBlowFishKey([]byte("0123456789abcdef"))
}

func TestGameServerLinkFreshRegistrationAndStatus(t *testing.T) {
	addr, _, servers, _, _ := newTestLink(t, true)
	// Seed id 1 as already registered (as if from a prior boot's DB load)
	// so this path never touches the nil DB stores.
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)

	ok, id, name, _ := gs.readAuthResult()
	if !ok || id != 1 || name != "Bartz" {
		t.Fatalf("readAuthResult() = ok=%v id=%d name=%q, want ok=true id=1 name=Bartz", ok, id, name)
	}

	entry, exists := servers.Get(1)
	if !exists || !entry.Authed || entry.Port != 7777 || entry.MaxPlayers != 300 {
		t.Fatalf("registry entry after auth = %+v", entry)
	}

	gs.sendServerStatus(map[int32]int32{7: 42}) // MAX_PLAYERS attribute
	time.Sleep(50 * time.Millisecond)
	entry, _ = servers.Get(1)
	if entry.MaxPlayers != 42 {
		t.Fatalf("MaxPlayers after ServerStatus = %d, want 42", entry.MaxPlayers)
	}

	gs.sendPlayerInGame("acc1", "acc2")
	time.Sleep(50 * time.Millisecond)
	if got := servers.OnlineAccountCount(1); got != 2 {
		t.Fatalf("OnlineAccountCount() = %d, want 2", got)
	}

	gs.sendPlayerLogout("acc1")
	time.Sleep(50 * time.Millisecond)
	if got := servers.OnlineAccountCount(1); got != 1 {
		t.Fatalf("OnlineAccountCount() after logout = %d, want 1", got)
	}
}

func TestGameServerLinkWrongHexIDRejected(t *testing.T) {
	addr, _, servers, _, _ := newTestLink(t, false)
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, []byte{0xff, 0xff})

	ok, _, _, reason := gs.readAuthResult()
	if ok || reason != byte(link.ReasonWrongHexID) {
		t.Fatalf("readAuthResult() = ok=%v reason=%d, want ok=false reason=%d", ok, reason, link.ReasonWrongHexID)
	}
}

func TestGameServerLinkAlreadyLoggedInRejected(t *testing.T) {
	addr, _, servers, _, _ := newTestLink(t, false)
	servers.Register(1, testHexID)

	first := dialGameServer(t, addr)
	first.handshake()
	first.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := first.readAuthResult(); !ok {
		t.Fatal("first registration failed, want success")
	}

	second := dialGameServer(t, addr)
	second.handshake()
	second.sendGameServerAuth(1, false, false, "*", 7778, 300, testHexID)
	ok, _, _, reason := second.readAuthResult()
	if ok || reason != byte(link.ReasonAlreadyLoggedIn) {
		t.Fatalf("second readAuthResult() = ok=%v reason=%d, want ok=false reason=%d", ok, reason, link.ReasonAlreadyLoggedIn)
	}
}

func TestGameServerLinkDisconnectMarksOffline(t *testing.T) {
	addr, _, servers, _, _ := newTestLink(t, false)
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("registration failed, want success")
	}

	gs.conn.Close()
	time.Sleep(100 * time.Millisecond)

	entry, _ := servers.Get(1)
	if entry.Authed {
		t.Fatal("entry.Authed = true after disconnect, want false")
	}
}

func TestGameServerLinkBannedIPRejected(t *testing.T) {
	addr, _, _, _, bans := newTestLink(t, false)
	bans.Ban(net.ParseIP("127.0.0.1"), 0)

	gs := dialGameServer(t, addr)
	payload := gs.readFrame()
	if payload[0] != link.OpcodeLoginServerFail || payload[1] != byte(link.ReasonIPBanned) {
		t.Fatalf("first packet = %v, want LoginServerFail(IPBanned)", payload)
	}
	gs.expectClosed()
}

// TestGameServerLinkRecoversFromConnectionHandlerPanic sends a
// GameServerAuth payload with a negative HexID length. wire.Reader's bounds
// check lets a negative n through (n > Remaining is false), so it slices
// with a high index below the low index and panics — this is a genuine
// malformed-payload panic, not a synthetic one, and it must disconnect only
// the offending link, not the whole login server.
func TestGameServerLinkRecoversFromConnectionHandlerPanic(t *testing.T) {
	addr, _, servers, _, _ := newTestLink(t, true)
	// Pre-seed id 2 (as if from a prior boot's DB load) so the second
	// connection's registration never touches the nil DB stores newTestLink
	// wires up for speed.
	servers.Register(2, testHexID)

	bad := dialGameServer(t, addr)
	bad.handshake()
	payload := []byte{link.OpcodeGameServerAuth, 1, 0, 0}
	payload = writeUTF16String(payload, "*")
	payload = binary.LittleEndian.AppendUint16(payload, 7777)
	payload = binary.LittleEndian.AppendUint32(payload, 300)
	negativeSize := int32(-1)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(negativeSize))
	bad.sendFrame(payload)
	bad.expectClosed()

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(2, false, false, "*", 7778, 300, testHexID)
	ok, id, _, _ := gs.readAuthResult()
	if !ok || id != 2 {
		t.Fatalf("readAuthResult() after recovering from panic = ok=%v id=%d, want ok=true id=2", ok, id)
	}
	if _, exists := servers.Get(2); !exists {
		t.Fatal("server 2 not registered after recovering from panic on a different connection")
	}
}

func TestGameServerLinkUnknownOpcodeAfterAuthCloses(t *testing.T) {
	addr, _, servers, _, _ := newTestLink(t, false)
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("registration failed, want success")
	}

	gs.sendFrame([]byte{0x7f})
	payload := gs.readFrame()
	if payload[0] != link.OpcodeLoginServerFail || payload[1] != byte(link.ReasonNotAuthed) {
		t.Fatalf("payload = %v, want LoginServerFail(NotAuthed)", payload)
	}
	gs.expectClosed()
}

func TestGameServerLinkPlayerAuthRequest(t *testing.T) {
	addr, _, servers, sessions, _ := newTestLink(t, false)
	servers.Register(1, testHexID)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendGameServerAuth(1, false, false, "*", 7777, 300, testHexID)
	if ok, _, _, _ := gs.readAuthResult(); !ok {
		t.Fatal("registration failed, want success")
	}

	key := link.SessionKey{PlayKey1: 1, PlayKey2: 2, LoginKey1: 3, LoginKey2: 4}

	// No session stored yet: must fail.
	gs.sendPlayerAuthRequest("acc1", key)
	if account, ok := gs.readPlayerAuthResponse(); account != "acc1" || ok {
		t.Fatalf("readPlayerAuthResponse() = %q, %v, want acc1, false", account, ok)
	}

	// A stored session with matching keys must succeed, once.
	sessions.Put("acc1", key)
	gs.sendPlayerAuthRequest("acc1", key)
	if account, ok := gs.readPlayerAuthResponse(); account != "acc1" || !ok {
		t.Fatalf("readPlayerAuthResponse() = %q, %v, want acc1, true", account, ok)
	}

	// The session was consumed: a second request must fail.
	gs.sendPlayerAuthRequest("acc1", key)
	if account, ok := gs.readPlayerAuthResponse(); account != "acc1" || ok {
		t.Fatalf("readPlayerAuthResponse() (replay) = %q, %v, want acc1, false", account, ok)
	}
}

func TestGameServerLinkRequiresAuthBeforeStatus(t *testing.T) {
	addr, _, _, _, _ := newTestLink(t, false)

	gs := dialGameServer(t, addr)
	gs.handshake()
	gs.sendServerStatus(map[int32]int32{7: 1})

	payload := gs.readFrame()
	if payload[0] != link.OpcodeLoginServerFail || payload[1] != byte(link.ReasonNotAuthed) {
		t.Fatalf("payload = %v, want LoginServerFail(NotAuthed)", payload)
	}
	gs.expectClosed()
}
