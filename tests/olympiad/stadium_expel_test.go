package olympiad

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestStadiumExpelsUninvitedPlayer pins the stadium's entry rule
// (OlympiadStadiumZone.onEnter): a player who is neither competitor,
// observer nor game master, here one entering the world inside a stadium,
// is sent to the town of its region, scattered by up to 20.
func TestStadiumExpelsUninvitedPlayer(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-500, 500, -500, 500, -1000, 1000)
	if err != nil {
		t.Fatalf("stadium form: %v", err)
	}
	stadium := zone.NewOlympiad(1, form)
	stadium.AddSpawn(zone.SpawnNormal, location.Location{X: 0, Y: 0, Z: gameservertest.SpawnZ})
	stadium.AddSpawn(zone.SpawnNormal, location.Location{X: 50, Y: 0, Z: gameservertest.SpawnZ})
	zones := zone.NewIndex()
	zones.Add(stadium)
	town := location.Location{X: 3000, Y: 3000, Z: gameservertest.SpawnZ}
	// The class spawn (10, 20) lies in map region (20, 18).
	restarts := &restart.Table{Points: []restart.Point{{Name: "town", Points: []location.Location{town}, MapRegions: []location.Point{{X: 20, Y: 18}}}}}

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Stray", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithRestartPoints(restarts),
	)
	burst := startInWorld(t, srv.Client)
	id := srv.SoleObjectID(t)
	var landed *location.Location
	for _, f := range append(burst, srv.ReadQueued(t, srv.Client)...) {
		if f[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != id {
			continue
		}
		at := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		landed = &at
	}
	if landed == nil {
		t.Fatal("the stray player was not teleported out of the stadium")
	}
	if dx, dy := landed.X-town.X, landed.Y-town.Y; dx < -20 || dx > 20 || dy < -20 || dy > 20 {
		t.Fatalf("stray player teleported to %+v, want within 20 of %+v", *landed, town)
	}
}
