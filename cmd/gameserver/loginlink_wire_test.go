package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport/linkpeer"
)

// TestOnLoginLinkRosterFollowsServerStatusOnTheWire registers with a login
// server that records the frames: the six-attribute ServerStatus goes first,
// then the roster of the accounts in the world as one PlayerInGame, as when
// the link comes up again with players online. An empty world sends no
// roster, so the next frame is whatever the game server sends next.
func TestOnLoginLinkRosterFollowsServerStatusOnTheWire(t *testing.T) {
	t.Parallel()
	for _, online := range []bool{true, false} {
		name := "empty world"
		if online {
			name = "players online"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			peer := linkpeer.Start(t, 1, "Bartz")
			status := network.NewLoginStatus(false, 300, func(link.ServerStatus) {})
			l, err := network.DialLoginLink(context.Background(), peer.Addr(), status.Auth(network.LoginServerAuth{
				ServerID: 1, HostName: "*", Port: 7777, MaxPlayers: 300,
			}), network.LoginLinkHandlers{}, zerolog.Nop())
			if err != nil {
				t.Fatalf("DialLoginLink: %v", err)
			}
			defer l.Close()

			state := world.New()
			if online {
				state.AddPlayer(&rosterPlayerStub{id: 1, account: "carol"})
			}
			onLoginLink(provideLoginLinkState(), status, state, filepath.Join(t.TempDir(), "hexid.txt"), []byte{1}, zerolog.Nop())(l)
			if err := l.SendPlayerInGame([]string{"dave"}); err != nil {
				t.Fatal(err)
			}

			if first := peer.Next(t); first[0] != link.OpcodeServerStatus {
				t.Fatalf("first frame opcode = %#x, want ServerStatus", first[0])
			}
			if online {
				roster, err := link.DecodePlayerInGame(peer.Next(t))
				if err != nil || len(roster) != 1 || roster[0] != "carol" {
					t.Fatalf("roster = %v, %v; want [carol]", roster, err)
				}
			}
			entering, err := link.DecodePlayerInGame(peer.Next(t))
			if err != nil || len(entering) != 1 || entering[0] != "dave" {
				t.Fatalf("next PlayerInGame = %v, %v; want [dave]", entering, err)
			}
		})
	}
}

// TestPlayerAuthAnswerWithoutWaitingClientSendsNoPlayerInGame answers an
// approval no client waits on (evicted or timed out): nothing goes to the
// login server, so the next frame on the wire is the one sent after it.
func TestPlayerAuthAnswerWithoutWaitingClientSendsNoPlayerInGame(t *testing.T) {
	t.Parallel()
	peer := linkpeer.Start(t, 1, "Bartz")
	l, err := network.DialLoginLink(context.Background(), peer.Addr(), network.LoginServerAuth{ServerID: 1, HostName: "*", Port: 7777, MaxPlayers: 300},
		network.LoginLinkHandlers{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	defer l.Close()

	answer := network.PlayerAuthAnswers(network.NewSessionValidator(), func() *network.LoginLink { return l }, zerolog.Nop())
	answer("ghost", true)
	if err := l.SendPlayerInGame([]string{"marker"}); err != nil {
		t.Fatal(err)
	}

	if first := peer.Next(t); first[0] != link.OpcodeServerStatus {
		t.Fatalf("first frame opcode = %#x, want ServerStatus", first[0])
	}
	next, err := link.DecodePlayerInGame(peer.Next(t))
	if err != nil || len(next) != 1 || next[0] != "marker" {
		t.Fatalf("frame after the unanswered approval = %v, %v; want only the marker", next, err)
	}
}
