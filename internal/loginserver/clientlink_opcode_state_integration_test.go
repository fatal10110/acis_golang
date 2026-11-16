package loginserver

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// Each connection state accepts only its own opcodes. Any other opcode,
// known or not, is dropped without a reply and the connection stays open:
// the next packet the state does accept is answered as usual. No second
// account can be authenticated on a connection that already logged in.
func TestClientLinkDropsOpcodesOutsideTheirState(t *testing.T) {
	hash := mustHashPassword(t, "s3cret")
	unknown := []byte{0x42, 0, 0, 0, 0}

	type state int
	const (
		connected state = iota // Init sent, no GameGuard exchange yet
		ggAuthed               // GGAuth sent, no login yet
		loggedIn               // LoginOk sent
	)
	tests := []struct {
		name    string
		state   state
		payload func(l *ClientLink, key1, key2 int32) []byte
	}{
		{"RequestAuthLogin before GameGuard", connected, func(l *ClientLink, _, _ int32) []byte {
			return encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret")
		}},
		{"RequestServerList before GameGuard", connected, func(*ClientLink, int32, int32) []byte { return encodeRequestServerList(0, 0) }},
		{"RequestServerLogin before GameGuard", connected, func(*ClientLink, int32, int32) []byte { return encodeRequestServerLogin(0, 0, 1) }},
		{"unknown opcode before GameGuard", connected, func(*ClientLink, int32, int32) []byte { return unknown }},
		{"AuthGameGuard twice", ggAuthed, func(*ClientLink, int32, int32) []byte { return encodeAuthGameGuard(testInitSessionID) }},
		{"RequestServerList before login", ggAuthed, func(*ClientLink, int32, int32) []byte { return encodeRequestServerList(0, 0) }},
		{"RequestServerLogin before login", ggAuthed, func(*ClientLink, int32, int32) []byte { return encodeRequestServerLogin(0, 0, 1) }},
		{"unknown opcode before login", ggAuthed, func(*ClientLink, int32, int32) []byte { return unknown }},
		{"AuthGameGuard after login", loggedIn, func(*ClientLink, int32, int32) []byte { return encodeAuthGameGuard(testInitSessionID) }},
		{"RequestAuthLogin for another account after login", loggedIn, func(l *ClientLink, _, _ int32) []byte {
			return encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "other", "s3cret")
		}},
		{"unknown opcode after login", loggedIn, func(*ClientLink, int32, int32) []byte { return unknown }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := newFakeAccountStore(
				model.NewAccount("player1", hash, 0, 1),
				model.NewAccount("other", hash, 0, 1),
			)
			addr, l, servers, sessions, _ := newTestClientLink(t, accounts, false)
			markOnlineAuto(t, servers, 1)
			c := dialLoginClient(t, addr)

			var key1, key2 int32
			switch tt.state {
			case ggAuthed:
				c.gameGuard()
			case loggedIn:
				key1, key2 = c.login(l, "player1", "s3cret")
			}

			c.send(tt.payload(l, key1, key2))

			// The dropped packet left no reply: the first frame read answers
			// the probe the state accepts.
			switch tt.state {
			case connected:
				c.gameGuard()
			case ggAuthed:
				c.authLogin(l, "player1", "s3cret")
			case loggedIn:
				c.send(encodeRequestServerList(key1, key2))
				if reply := c.read(); reply[0] != serverpackets.OpcodeServerList {
					t.Fatalf("opcode = %#x, want ServerList (%#x)", reply[0], serverpackets.OpcodeServerList)
				}
				if _, ok := sessions.Get("other"); ok {
					t.Fatal("a second account was authenticated on a logged-in connection")
				}
				if _, ok := accounts.getLastActive("other"); ok {
					t.Fatal("a second account's login was processed on a logged-in connection")
				}
			}
		})
	}
}

// TestClientLinkDroppedPacketsDoNotExtendHandshakeDeadline keeps a client
// that passed the GameGuard exchange sending packets the server drops
// unanswered, faster than the handshake timeout: the connection still closes
// once that timeout runs out.
func TestClientLinkDroppedPacketsDoNotExtendHandshakeDeadline(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload []byte
	}{
		// Too short to carry the credential block.
		{"truncated RequestAuthLogin", []byte{clientpackets.OpcodeRequestAuthLogin, 0, 0, 0, 0}},
		{"opcode outside the state", encodeRequestServerList(0, 0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			addr, _, _, _, _ := newTestClientLink(t, newFakeAccountStore(), false, func(l *ClientLink) {
				l.handshakeTimeout = 300 * time.Millisecond
			})
			c := dialLoginClient(t, addr)
			c.gameGuard()

			// One encrypted frame, written again and again: the session
			// cipher carries no state between frames.
			buf := make([]byte, commoncrypt.PaddedSize(len(tt.payload)+4))
			copy(buf, tt.payload)
			commoncrypt.AppendChecksum(buf)
			commoncrypt.EncryptBlocks(c.cipher, buf)
			var frame bytes.Buffer
			if err := wire.WriteFrame(&frame, buf); err != nil {
				t.Fatalf("WriteFrame: %v", err)
			}

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
						if _, err := c.conn.Write(frame.Bytes()); err != nil {
							return
						}
					}
				}
			}()
			defer func() { close(stop); <-done }()

			c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			b := make([]byte, 1)
			if n, err := c.conn.Read(b); n != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatalf("connection fed only dropped packets: n=%d err=%v, want closed", n, err)
			}
		})
	}
}
