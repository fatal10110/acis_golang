package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestDropItem.java:36 ignores a drop only from a dead
// player, so one playing dead still drops; the item lands where
// ItemInstance.dropMe puts it, at GeoEngine.getValidLocation from the
// dropper toward the requested point (ItemInstance.java:744-761).

// wallDropTarget is a requested drop point geodata does not let the item
// reach from the dropper; wallGeo stops it one step off the dropper,
// slightly lower.
var wallDropTarget = location.Location{X: spawnX + 60, Y: spawnY, Z: spawnZ}

// wallGeo is passable geodata with a wall between the dropper and
// wallDropTarget.
type wallGeo struct{ gameservertest.Geo }

func (wallGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if (location.Location{X: tx, Y: ty, Z: tz}) == wallDropTarget {
		return location.Location{X: ox + 1, Y: oy + 1, Z: oz - 3}
	}
	return location.Location{X: tx, Y: ty, Z: tz}
}

// soleGroundAt requires a single ground item, at want.
func soleGroundAt(t *testing.T, srv *gameservertest.Server, want location.Location) {
	t.Helper()
	snaps := srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("ground items = %+v, want one", snaps)
	}
	if got := (location.Location{X: snaps[0].X, Y: snaps[0].Y, Z: snaps[0].Z}); got != want {
		t.Fatalf("dropped item at %+v, want %+v", got, want)
	}
}

// TestDropStopsWhereGeodataAllows drops adena past a wall: it lands at the
// valid location geodata answers from the dropper's own position, not at
// the requested point.
func TestDropStopsWhereGeodataAllows(t *testing.T) {
	t.Parallel()
	srv, objID, adena := bootDropper(t, userAccessLevel, gameservertest.WithGeo(wallGeo{}))
	c := srv.Client

	c.Send(encodeRequestDropItem(adena, 40, int32(wallDropTarget.X), int32(wallDropTarget.Y), int32(wallDropTarget.Z)))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeDropItem, "DropItem")
	soleGroundAt(t, srv, location.Location{X: spawnX + 1, Y: spawnY + 1, Z: spawnZ - 3})
	assertItemCount(t, srv, objID, adena, 60)
}

// TestDropOnOpenGroundLandsAtRequestedPoint: with nothing in the way the
// item lands exactly where the client asked.
func TestDropOnOpenGroundLandsAtRequestedPoint(t *testing.T) {
	t.Parallel()
	srv, _, adena := bootDropper(t, userAccessLevel, gameservertest.WithGeo(wallGeo{}))
	c := srv.Client
	want := location.Location{X: spawnX + 40, Y: spawnY + 10, Z: spawnZ}

	c.Send(encodeRequestDropItem(adena, 40, int32(want.X), int32(want.Y), int32(want.Z)))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeDropItem, "DropItem")
	soleGroundAt(t, srv, want)
}

// TestDropWhileFakeDeadLands: a player playing dead still drops.
func TestDropWhileFakeDeadLands(t *testing.T) {
	t.Parallel()
	srv, objID, adena := bootDropper(t, userAccessLevel)
	c := srv.Client
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !pc.StartFakeDeath() {
		t.Fatal("StartFakeDeath reported no change")
	}
	drainUntilQuiet(t, c)

	assertDropLands(t, c, adena, 40)
	soleGroundAt(t, srv, location.Location{X: spawnX, Y: spawnY, Z: spawnZ})
	assertItemCount(t, srv, objID, adena, 60)
	if !pc.FakeDead() {
		t.Fatal("the drop ended the fake death")
	}
}

// TestDropWhileDeadIsIgnored: a dead player's drop is ignored without an
// answer and leaves the item held.
func TestDropWhileDeadIsIgnored(t *testing.T) {
	t.Parallel()
	srv, objID, adena := bootDropper(t, userAccessLevel)
	c := srv.Client
	srv.MarkPlayerDead(t, objID)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))
	assertNoFrame(t, c, "after a dead player's drop")
	if snaps := srv.GroundItems.Snapshots(nil); len(snaps) != 0 {
		t.Fatalf("ground items after a dead player's drop = %+v, want none", snaps)
	}
	assertItemCount(t, srv, objID, adena, 100)
}
