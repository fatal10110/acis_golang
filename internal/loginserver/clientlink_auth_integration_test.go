package loginserver

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

func TestClientLinkSendsInitOnConnect(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	dialLoginClient(t, addr) // dial fails the test itself if Init never arrives
}

func TestClientLinkAuthGameGuardRepliesGGAuth(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	c := dialLoginClient(t, addr)

	c.send(encodeAuthGameGuard(testInitSessionID))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeGGAuth {
		t.Fatalf("opcode = %#x, want GGAuth (%#x)", reply[0], serverpackets.OpcodeGGAuth)
	}
	if got := int32(binary.LittleEndian.Uint32(reply[1:5])); got != testInitSessionID {
		t.Fatalf("GGAuth response = %#x, want the Init session id %#x", got, testInitSessionID)
	}
}

func TestClientLinkAuthGameGuardClosesOnGGAuthWriteFailure(t *testing.T) {
	server, client := net.Pipe()
	client.Close()
	crypt, err := logincrypt.NewLoginCrypt(testSessionKey)
	if err != nil {
		t.Fatalf("NewLoginCrypt: %v", err)
	}
	c := &clientConn{conn: server, crypt: crypt, sessionID: testInitSessionID}
	defer server.Close()

	if (&ClientLink{}).onAuthGameGuard(c, encodeAuthGameGuard(testInitSessionID)) {
		t.Fatal("onAuthGameGuard() = true after GGAuth write failure, want false")
	}
	if c.ggAuthed {
		t.Fatal("ggAuthed = true after GGAuth write failure")
	}
}

func TestClientLinkGameGuardWrongSessionIDClosesWithAccessFailed(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	c := dialLoginClient(t, addr)

	c.send(encodeAuthGameGuard(testInitSessionID + 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccessFailed {
		t.Fatalf("reason = %d, want REASON_ACCESS_FAILED (%d)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()
}

func TestClientLinkCredentialsBeforeGameGuardAreDropped(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))

	if _, ok := sessions.Get("player1"); ok {
		t.Fatal("credentials sent before the GameGuard exchange must not create a session")
	}

	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginOk {
		t.Fatalf("opcode = %#x, want LoginOk (%#x)", reply[0], serverpackets.OpcodeLoginOk)
	}
}

func TestClientLinkLoginSuccess(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.gameGuard()
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

	if _, ok := accounts.getLastActive("player1"); !ok {
		t.Fatal("expected last-active time to be recorded on successful login")
	}
}

func TestClientLinkLoginDecodeFailureSendsAccessFailed(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false)
	c := dialLoginClient(t, addr)

	c.gameGuard()

	// A RequestAuthLogin body shorter than the 128-byte RSA credential block
	// fails to decode before any account lookup happens.
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAuthLogin)
	w.WriteBytes(make([]byte, 16))
	c.send(w.Bytes())

	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccessFailed {
		t.Fatalf("reason = %d, want REASON_ACCESS_FAILED (%d)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()
}

func TestClientLinkLoginLastActiveWriteFailureSendsAccessFailed(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	accounts.lastActiveErr = errors.New("db unavailable")
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailAccessFailed {
		t.Fatalf("reason = %d, want REASON_ACCESS_FAILED (%d)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()

	if _, ok := sessions.Get("player1"); ok {
		t.Fatal("expected no session to be stored when the last-active write fails")
	}
}

func TestClientLinkLoginWrongPassword(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailPasswordWrong {
		t.Fatalf("reason = %d, want REASON_PASS_WRONG (%d)", reason, serverpackets.LoginFailPasswordWrong)
	}
	c.expectClosed()
}

func TestClientLinkLoginUnknownAccountAutoCreateOff(t *testing.T) {
	accounts := newFakeAccountStore()
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "newplayer", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := loginFailReason(t, reply); reason != serverpackets.LoginFailUserOrPassWrong {
		t.Fatalf("reason = %d, want REASON_USER_OR_PASS_WRONG (%d)", reason, serverpackets.LoginFailUserOrPassWrong)
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

	c.gameGuard()
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

// Test links hash auto-created passwords at MinCost; the production
// constructor must keep hashing at the full default cost.
func TestNewClientLinkHashesAutoCreatedPasswordsAtDefaultCost(t *testing.T) {
	l := NewClientLink(nil, nil, nil, nil, nil, nil, true, true, DefaultLoginTryBeforeBan, DefaultLoginBlockAfterBan, zerolog.Nop())
	hashed, err := l.hashPassword("s3cret")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hashed))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("auto-create hash cost = %d, want %d", cost, bcrypt.DefaultCost)
	}
}

func TestClientLinkLoginBannedAccountRejected(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("banned", mustHashPassword(t, "s3cret"), -1, 1))
	addr, l, _, _, _ := newTestClientLink(t, accounts, false)
	c := dialLoginClient(t, addr)

	c.gameGuard()
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
	c.gameGuard()
	c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	c.expectClosed()
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
