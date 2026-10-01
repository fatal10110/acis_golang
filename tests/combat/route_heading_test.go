package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// doglegGeo is open ground where only axis-aligned straight lines pass: any
// diagonal line is closed, and the pathfinder routes it as a dogleg, first
// along Y to the target's row, then along X to the target.
type doglegGeo struct{ gameservertest.Geo }

func (doglegGeo) CanMove(ox, oy, _, tx, ty, _ int) bool { return ox == tx || oy == ty }

func (doglegGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return []location.Location{{X: origin.X, Y: target.Y, Z: target.Z}, target}, true
}

func encodeCannotMoveAnymore(x, y, z, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCannotMoveAnymore)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(heading)
	return w.Bytes()
}

// stopMoveHeading reads a StopMove frame's object and heading.
func stopMoveHeading(t *testing.T, frame []byte) (int32, int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStopMove, "StopMove")
	r := wireReader(frame[1:])
	id := r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	r.ReadInt32()
	return id, int(r.ReadInt32())
}

// TestRoutedGroundClickWalkFacesFirstLeg pins a player's heading on a routed
// walk start: PlayableAI.thinkMoveTo path-finds the click
// (maybeMoveToLocation(loc, 0, true, false)), and PlayerMove.moveToLocation
// replaces the destination with the first geopath leg before
// setHeadingTo(destination) and the MoveToLocation broadcast. A stop before
// the first segment advance reports that leg's direction, not the click's;
// a straight walk faces the click.
func TestRoutedGroundClickWalkFacesFirstLeg(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		click location.Location
		leg   location.Location
	}{
		{
			name:  "routed",
			click: location.Location{X: playerOrigin.X + 800, Y: playerOrigin.Y + 600, Z: playerOrigin.Z},
			leg:   location.Location{X: playerOrigin.X, Y: playerOrigin.Y + 600, Z: playerOrigin.Z},
		},
		{
			name:  "straight",
			click: location.Location{X: playerOrigin.X + 800, Y: playerOrigin.Y, Z: playerOrigin.Z},
			leg:   location.Location{X: playerOrigin.X + 800, Y: playerOrigin.Y, Z: playerOrigin.Z},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithGeo(doglegGeo{}),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			drainUntilQuiet(t, c)

			c.Send(encodeMoveBackwardToLocation(int32(tc.click.X), int32(tc.click.Y), int32(tc.click.Z)))
			id, dest, origin := moveToLocationCoords(t, readUntil(t, c, serverpackets.OpcodeMoveToLocation, "walk MoveToLocation")[0])
			if id != objID || dest != tc.leg {
				t.Fatalf("MoveToLocation object %d dest %+v, want %d walking the first leg %+v", id, dest, objID, tc.leg)
			}
			want := origin.HeadingTo(tc.leg)
			if tc.name == "routed" && want == origin.HeadingTo(tc.click) {
				t.Fatalf("fixture: first leg %+v and click %+v share heading %d", tc.leg, tc.click, want)
			}

			c.Send(encodeCannotMoveAnymore(int32(origin.X), int32(origin.Y), int32(origin.Z), 0))
			id, got := stopMoveHeading(t, readUntil(t, c, serverpackets.OpcodeStopMove, "StopMove")[0])
			if id != objID || got != want {
				t.Fatalf("StopMove object %d heading %d, want %d heading %d toward the first leg (click heading %d)",
					id, got, objID, want, origin.HeadingTo(tc.click))
			}
		})
	}
}

// TestRoutedFearFleeFacesFirstLeg pins a player's flee heading on a routed
// run: Creature.fleeFrom walks by maybeMoveToLocation(loc, 0, true, false),
// so the run faces its first geopath leg, not the flee point.
func TestRoutedFearFleeFacesFirstLeg(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(doglegGeo{}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	// Off both axes of the player, so the run away from it is diagonal.
	home := location.Location{X: hostileX, Y: hostileY + 50, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(interface {
		effectHolder
		Heading() int
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder with a heading", objID, obj)
	}
	origin := location.Location{}
	origin.X, origin.Y, origin.Z = player.Position()
	flee := origin.FleeFrom(home.X, home.Y, fearFleeDistance)
	leg := location.Location{X: origin.X, Y: flee.Y, Z: origin.Z}
	if origin.HeadingTo(leg) == origin.HeadingTo(flee) {
		t.Fatalf("fixture: first leg %+v and flee point %+v share a heading", leg, flee)
	}

	landFear(t, hostile, player, curseFearSkillID, 10)
	if dest := readMoveOf(t, c, objID, "landing flee"); dest.X != leg.X || dest.Y != leg.Y {
		t.Fatalf("landing flee dest = %+v, want the first leg %+v toward %+v", dest, leg, flee)
	}
	if got, want := player.Heading(), origin.HeadingTo(leg); got != want {
		t.Fatalf("Heading() = %d after the routed flee start, want %d toward the first leg (flee point %d)",
			got, want, origin.HeadingTo(flee))
	}
}
