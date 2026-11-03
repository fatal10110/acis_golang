package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The drops below pin the DROP_ITEM reuse gate every RequestDropItem spends
// before any other check: DropItemTime per client (server.properties,
// stock 1000 ms), restarted by each drop that passes the gate whatever
// happens to it next, and silent when it refuses.

const dropItemTime = time.Second

// bootDropFlood boots the dropper with DropItemTime set on the driven clock.
func bootDropFlood(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv, objID, adena := bootDropper(t, userAccessLevel, gameservertest.WithDropItemDelay(dropItemTime))
	if !srv.DrivesClock() {
		t.Skip("placing drops on either side of the reuse window needs the driven clock")
	}
	drainUntilQuiet(t, srv.Client)
	return srv, objID, adena
}

// advanceDropClockTo lets the driven clock reach at, as the client reads
// it, and drops whatever the wait sent.
func advanceDropClockTo(t *testing.T, srv *gameservertest.Server, at time.Time) {
	t.Helper()
	if d := at.Sub(srv.Client.Now()); d > 0 {
		srv.Advance(t, d)
	}
	drainUntilQuiet(t, srv.Client)
}

// assertDropIgnored sends a drop of count adena and requires no answer at
// all, the stack untouched and nothing more on the ground.
func assertDropIgnored(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, ownerID, objectID, count int32, grounded int, what string) {
	t.Helper()
	srv.FlushItems(t)
	before := mustFindItem(t, srv, ownerID, objectID).Count
	c.Send(encodeRequestDropItem(objectID, count, spawnX, spawnY, spawnZ))
	if queued := c.ReadQueued(); len(queued) != 0 {
		t.Fatalf("%s: got opcode %#x, want no answer", what, queued[0][0])
	}
	if snaps := srv.GroundItems.Snapshots(nil); len(snaps) != grounded {
		t.Fatalf("%s: %d ground items, want %d", what, len(snaps), grounded)
	}
	srv.FlushItems(t)
	if got := mustFindItem(t, srv, ownerID, objectID).Count; got != before {
		t.Fatalf("%s: stack count = %d, want %d", what, got, before)
	}
}

func TestDropInsideDropItemTimeIgnored(t *testing.T) {
	t.Parallel()
	srv, objID, adena := bootDropFlood(t)
	c := srv.Client

	start := c.Now()
	assertDropLands(t, c, adena, 10)
	drainUntilQuiet(t, c)

	assertDropIgnored(t, srv, c, objID, adena, 10, 1, "drop right after the first")
	advanceDropClockTo(t, srv, start.Add(dropItemTime-10*time.Millisecond))
	assertDropIgnored(t, srv, c, objID, adena, 10, 1, "drop just short of DropItemTime")

	// The ignored drops did not push the window out.
	advanceDropClockTo(t, srv, start.Add(dropItemTime))
	assertDropLands(t, c, adena, 10)
	drainUntilQuiet(t, c)
	if snaps := srv.GroundItems.Snapshots(nil); len(snaps) != 2 {
		t.Fatalf("%d ground items after the drop past DropItemTime, want 2", len(snaps))
	}
}

func TestDropRefusedPastTheGateStillStartsDropItemTime(t *testing.T) {
	t.Parallel()
	srv, objID, adena := bootDropFlood(t)
	c := srv.Client

	// Out of reach: refused with its message, after the gate was spent.
	start := c.Now()
	assertDropRefused(t, srv, c, objID, adena, 10, spawnX+1000, serverpackets.SystemMessageCannotDiscardDistanceTooFar)
	assertDropIgnored(t, srv, c, objID, adena, 10, 0, "drop in reach inside the refused drop's DropItemTime")

	advanceDropClockTo(t, srv, start.Add(dropItemTime))
	assertDropLands(t, c, adena, 10)
}

func TestDropItemTimeIsPerClient(t *testing.T) {
	t.Parallel()
	srv, _, adena := bootDropFlood(t)
	c := srv.Client
	other := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	otherAdena := srv.GiveItem(t, other.ID, item.AdenaID, 100)
	oc := srv.DialClient(t, "player2", 1)
	startInWorld(t, oc)
	drainUntilQuiet(t, oc)
	drainUntilQuiet(t, c)

	assertDropLands(t, c, adena, 10)
	drainUntilQuiet(t, oc)
	assertDropLands(t, oc, otherAdena, 10)
	if snaps := srv.GroundItems.Snapshots(nil); len(snaps) != 2 {
		t.Fatalf("%d ground items after both players dropped, want 2", len(snaps))
	}
}
