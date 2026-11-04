package lifecycle

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The game-client port refuses connection bursts from one address at
// accept: within a second of the previous connection each one counts a
// try, the fourth try bans the address, and only a sweep after 300 s of
// silence lifts the ban. The filter reads a fake clock the suite sets, and
// the suite runs its sweeps, so no wall-clock time passes.

const protocolRevision = 746

// filterClock is the fake clock the accept filter reads.
type filterClock struct{ nanos atomic.Int64 }

func (c *filterClock) now() time.Time   { return time.Unix(0, c.nanos.Load()) }
func (c *filterClock) set(at time.Time) { c.nanos.Store(at.UnixNano()) }
func (c *filterClock) advance(d time.Duration) time.Time {
	at := c.now().Add(d)
	c.set(at)
	return at
}

// bootFiltered boots a server whose game-client listener runs f on clock;
// the harness's own client is the address's first connection.
func bootFiltered(t *testing.T) (*gameservertest.Server, *netutil.AcceptFilter, *filterClock) {
	t.Helper()
	clock := &filterClock{}
	clock.set(time.Unix(1_000_000, 0))
	f := netutil.NewAcceptFilter()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1),
		gameservertest.WithAcceptFilter(f, clock.now))
	return srv, f, clock
}

// assertHandshake connects from local and requires the VersionCheck answer
// to ProtocolVersion.
func assertHandshake(t *testing.T, srv *gameservertest.Server, local string) {
	t.Helper()
	testsupport.DialFrom(t, srv.Addr(), local).SendProtocolVersion(protocolRevision)
}

// assertRefused connects from local, sends ProtocolVersion and requires the
// connection to close with no byte answered.
func assertRefused(t *testing.T, srv *gameservertest.Server, local string) {
	t.Helper()
	c := testsupport.DialFrom(t, srv.Addr(), local)
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(protocolRevision)
	_ = wire.WriteFrame(c.Conn(), w.Bytes()) // the server may already have closed
	c.ExpectClosed()
}

func TestAcceptFilterBansBurstUntilSweep(t *testing.T) {
	t.Parallel()
	srv, f, clock := bootFiltered(t)

	for i := 2; i <= 4; i++ {
		clock.advance(100 * time.Millisecond)
		assertHandshake(t, srv, "127.0.0.1")
	}
	clock.advance(100 * time.Millisecond)
	assertRefused(t, srv, "127.0.0.1")

	// Each refused attempt under a second after the previous one keeps the
	// ban, as does a slower one: the record is still there.
	clock.advance(900 * time.Millisecond)
	assertRefused(t, srv, "127.0.0.1")
	clock.advance(999 * time.Millisecond)
	assertRefused(t, srv, "127.0.0.1")
	clock.advance(5 * time.Second)
	assertRefused(t, srv, "127.0.0.1")

	// Another host is unaffected.
	assertHandshake(t, srv, "127.0.0.2")

	// A sweep at exactly 300 s of silence keeps the ban; one past it lifts it.
	f.Sweep(clock.advance(300 * time.Second))
	assertRefused(t, srv, "127.0.0.1")
	f.Sweep(clock.advance(300*time.Second + time.Millisecond))
	assertHandshake(t, srv, "127.0.0.1")
}

func TestAcceptFilterSlowConnectionKeepsTries(t *testing.T) {
	t.Parallel()
	srv, _, clock := bootFiltered(t)

	for i := 2; i <= 4; i++ {
		clock.advance(10 * time.Millisecond)
		assertHandshake(t, srv, "127.0.0.1")
	}
	// A full second after the previous connection: admitted, but the three
	// tries stay, so the next burst connection is refused.
	clock.advance(time.Second)
	assertHandshake(t, srv, "127.0.0.1")
	clock.advance(10 * time.Millisecond)
	assertRefused(t, srv, "127.0.0.1")
}
