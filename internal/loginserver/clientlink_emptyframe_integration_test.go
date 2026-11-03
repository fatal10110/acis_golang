package loginserver

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// emptyFrames is three frames with no body: length headers 0 and 1 (below
// the header's own size) and 2 (the header alone).
var emptyFrames = []byte{0x00, 0x00, 0x01, 0x00, 0x02, 0x00}

func (f *fakeLoginClient) sendEmptyFrames() {
	f.t.Helper()
	if _, err := f.conn.Write(emptyFrames); err != nil {
		f.t.Fatalf("write empty frames: %v", err)
	}
}

// TestClientLinkSkipsEmptyFrames interleaves frames with no body through the
// whole login flow: each is skipped without a reply, and the request after
// it is answered as if it had not been sent.
func TestClientLinkSkipsEmptyFrames(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 2))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	markOnlineAuto(t, servers, 7)

	c := dialLoginClient(t, addr)
	c.sendEmptyFrames()
	c.gameGuard()
	c.sendEmptyFrames()
	key1, key2 := c.login(l, "player1", "s3cret")
	c.sendEmptyFrames()

	c.send(encodeRequestServerList(key1, key2))
	if reply := c.read(); reply[0] != serverpackets.OpcodeServerList {
		t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
	}
}

// TestClientLinkEmptyFramesDoNotExtendHandshakeDeadline keeps a client that
// never completes the handshake sending empty frames faster than the
// handshake timeout: the connection still closes once that timeout runs out.
func TestClientLinkEmptyFramesDoNotExtendHandshakeDeadline(t *testing.T) {
	addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false, func(l *ClientLink) {
		l.handshakeTimeout = 300 * time.Millisecond
	})

	c := dialLoginClient(t, addr)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if _, err := c.conn.Write(emptyFrames); err != nil {
					return
				}
			}
		}
	}()
	defer func() { close(stop); <-done }()

	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	if n, err := c.conn.Read(buf); n != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("connection fed only empty frames: n=%d err=%v, want closed", n, err)
	}
}

// TestClientLinkPurgeClosesAuthedClientSendingEmptyFrames requires the purge
// loop to close an authenticated connection past its login timeout even
// though empty frames arrived after authentication.
func TestClientLinkPurgeClosesAuthedClientSendingEmptyFrames(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, sessions, _ := newTestClientLink(t, accounts, false, func(l *ClientLink) { l.loginTimeout = 300 * time.Millisecond })

	c := dialLoginClient(t, addr)
	c.login(l, "player1", "s3cret")
	for range 5 {
		c.sendEmptyFrames()
	}

	reply := c.read()
	if reply[0] != serverpackets.OpcodeLoginFail {
		t.Fatalf("opcode = %#x, want LoginFail (%#x)", reply[0], serverpackets.OpcodeLoginFail)
	}
	if reason := reply[1]; reason != byte(serverpackets.LoginFailAccessFailed) {
		t.Fatalf("LoginFail reason = %#x, want AccessFailed (%#x)", reason, serverpackets.LoginFailAccessFailed)
	}
	c.expectClosed()
	waitSessionMissing(t, sessions, "player1")
}

// TestGameServerLinkClosesOnEmptyFrame keeps the game-server link strict: a
// frame with no body, whether its length header is below the header's own
// size or exactly the header, closes the link.
func TestGameServerLinkClosesOnEmptyFrame(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header []byte
	}{
		{"length 1", []byte{0x01, 0x00}},
		{"length 2", []byte{0x02, 0x00}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, _, _, _, _ := newTestLink(t, true)
			gs := dialGameServer(t, addr)
			gs.handshake()
			if _, err := gs.conn.Write(tc.header); err != nil {
				t.Fatalf("write header: %v", err)
			}
			gs.expectClosed()
		})
	}
}
