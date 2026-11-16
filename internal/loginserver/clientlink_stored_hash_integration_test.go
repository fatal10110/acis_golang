package loginserver

import (
	"net"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// Hashes the reference server produced for password "s3cret".
const (
	refStoredHash2a = "$2a$04$Ro0CUfOqk6cXEKf3dyaM7OpNNVVUxK2/c53ykg4bn8cFRHYdF4UzO"
	refStoredHash2  = "$2$04$Ro0CUfOqk6cXEKf3dyaM7O7fMbZhCvrR98qQgvC8MBoT7GmQXPRya"
)

// A stored hash the server cannot check answers every attempt with
// LoginFail ACCESS_FAILED and never counts toward the IP ban, whatever
// password the client sends.
func TestClientLinkUnusableStoredHashFailsWithoutCountingAttempt(t *testing.T) {
	for name, hash := range map[string]string{
		"2b":        "$2b$" + refStoredHash2a[4:],
		"2y":        "$2y$" + refStoredHash2a[4:],
		"legacy":    "5f4dcc3b5aa765d61d8327deb882cf99",
		"truncated": refStoredHash2a[:20],
	} {
		t.Run(name, func(t *testing.T) {
			accounts := newFakeAccountStore(model.NewAccount("player1", hash, 0, 1))
			addr, l, _, _, bans := newTestClientLink(t, accounts, false)
			ip := net.ParseIP("127.0.0.1")

			for range DefaultLoginTryBeforeBan + 1 {
				c := dialLoginClient(t, addr)
				c.gameGuard()
				c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "s3cret"))
				c.expectLoginFail(serverpackets.LoginFailAccessFailed)
			}
			if bans.IsBanned(ip) {
				t.Fatal("unusable stored hash counted toward the IP ban")
			}
		})
	}
}

// Reference-produced "$2a$" and "$2$" hashes log in with the right password.
func TestClientLinkReferenceStoredHashesLogIn(t *testing.T) {
	for name, hash := range map[string]string{"2a": refStoredHash2a, "2": refStoredHash2} {
		t.Run(name, func(t *testing.T) {
			accounts := newFakeAccountStore(model.NewAccount("player1", hash, 0, 1))
			addr, l, _, _, _ := newTestClientLink(t, accounts, false)
			dialLoginClient(t, addr).login(l, "player1", "s3cret")
		})
	}
}

// The server list carries servers in ascending id order on every request,
// whatever order they were registered in.
func TestClientLinkServerListAscendingByID(t *testing.T) {
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, servers, _, _ := newTestClientLink(t, accounts, false)
	for _, id := range []int{9, 3, 7} {
		servers.Register(id, []byte{byte(id)})
		servers.MarkOnline(id, "127.0.0.1", net.ParseIP("127.0.0.1"), 7777, 100)
	}

	const entrySize = 21
	c := dialLoginClient(t, addr)
	key1, key2 := c.login(l, "player1", "s3cret")
	for range 5 {
		c.send(encodeRequestServerList(key1, key2))
		reply := c.read()
		if reply[0] != serverpackets.OpcodeServerList || reply[1] != 3 {
			t.Fatalf("reply opcode %#x count %d, want ServerList with 3 servers", reply[0], reply[1])
		}
		var ids []byte
		for i := range 3 {
			ids = append(ids, reply[3+i*entrySize])
		}
		if want := []byte{3, 7, 9}; !slices.Equal(ids, want) {
			t.Fatalf("server ids = %v, want %v", ids, want)
		}
	}
}
