package lifecycle

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// bootConfigured boots a server whose game-client listener runs the accept
// filter that the server.properties text props configures, on a fake clock.
func bootConfigured(t *testing.T, props string) (*gameservertest.Server, *filterClock) {
	t.Helper()
	p, err := config.ParseString(props)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.AcceptFilter(p)
	if err != nil {
		t.Fatal(err)
	}
	clock := &filterClock{}
	clock.set(time.Unix(1_000_000, 0))
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1),
		gameservertest.WithAcceptFilter(netutil.NewAcceptFilter(cfg, zerolog.Nop()), clock.now))
	return srv, clock
}

// TestAcceptFilterRaisedLimitAdmitsTheDefaultBannedBurst sends one burst,
// eleven connections 100 ms apart from one host, to a server on the
// default filter and to one whose server.properties raises the burst limit
// to ten: the default refuses the fifth, the raised limit admits all
// eleven and refuses the twelfth. An allowlisted host is never refused.
func TestAcceptFilterRaisedLimitAdmitsTheDefaultBannedBurst(t *testing.T) {
	t.Parallel()
	const burster, loadHost = "127.0.0.2", "127.0.0.3"
	t.Run("default", func(t *testing.T) {
		t.Parallel()
		srv, clock := bootConfigured(t, "")
		for range 4 {
			assertHandshake(t, srv, burster)
			clock.advance(100 * time.Millisecond)
		}
		assertRefused(t, srv, burster)
	})
	t.Run("raised", func(t *testing.T) {
		t.Parallel()
		srv, clock := bootConfigured(t, "AcceptFilterBurstLimit = 10\nAcceptFilterAllowlist = 127.0.0.3/32\n")
		for range 11 {
			assertHandshake(t, srv, burster)
			clock.advance(100 * time.Millisecond)
		}
		assertRefused(t, srv, burster)
		for range 20 {
			assertHandshake(t, srv, loadHost)
			clock.advance(time.Millisecond)
		}
	})
}
