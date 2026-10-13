package boat

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// TestAdminReloadBoatTellsAfterTheNewBoats pins the order a game master on
// the dock sees //reload boat in while another passenger is aboard: the old
// boat goes and the new one comes, as one reload, before the game master is
// told the boats reloaded, and the next type on the command line reloads
// only then.
func TestAdminReloadBoatTellsAfterTheNewBoats(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, _ := bootReload(t, []route.BoatItinerary{reloadedItinerary()}, &fail, passengerItinerary())
	oldBoat := srv.Boats.Boats()[0]
	old := oldBoat.ObjectID()
	rider, riderID := addRider(t, srv, "rider", "Rider", runeBoarding)
	board(t, srv, rider, old)
	srv.ReadQueued(t, gm)

	x, y, z := oldBoat.Position()
	stoppedAt := location.Location{X: x, Y: y, Z: z}
	gm.Send(encodeBuildCmd("reload boat nothing"))
	log := reloadLog(t, srv, gm)
	reloadLog(t, srv, rider)
	boats := srv.Boats.Boats()
	if len(boats) != 1 {
		t.Fatalf("fleet after the reload = %d boats, want 1", len(boats))
	}
	assertLog(t, "game master on the dock", log, append([]string{
		started(old, 0), info(old, stoppedAt, oldBoat.Heading()),
		teleport(riderID, reloadTown), deleted(riderID),
		deleted(old), info(boats[0].ObjectID(), runeDock, 1000),
		reloadDone,
	}, reloadUsage...)...)
}
