package loginserver

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
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
