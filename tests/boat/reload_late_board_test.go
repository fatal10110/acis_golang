package boat

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminReloadBoatRefusesLateBoarding holds a passenger's queue so
// //reload boat keeps the retired boat in the world, then has a player on
// the dock board it: the boarding is refused, so no one is left aboard a
// boat that leaves the world. Once the passenger is ashore the old boat
// goes, and the player boards the new one as from any dock.
func TestAdminReloadBoatRefusesLateBoarding(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, _ := bootReloadWith(t, []route.BoatItinerary{reloadedItinerary()}, &fail,
		[]route.BoatItinerary{passengerItinerary()}, gameservertest.WithRealPool())
	oldBoat := srv.Boats.Boats()[0]
	old := oldBoat.ObjectID()
	rider, riderID := addRider(t, srv, "rider", "Rider", runeBoarding)
	late, lateID := addRider(t, srv, "late", "Late", runeBoarding)
	board(t, srv, rider, old)
	srv.ReadQueued(t, gm)
	srv.ReadQueued(t, late)

	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	letGo := func() { once.Do(func() { close(release) }) }
	t.Cleanup(letGo)
	if !srv.PlayerQueue(t, riderID).Post(func() { close(held); <-release }) {
		t.Fatal("post to the passenger's queue: queue closed")
	}
	<-held
	gm.Send(encodeBuildCmd("reload boat"))
	srv.AwaitHandled(t)
	if _, ok := srv.State.Object(old); !ok {
		t.Fatal("the old boat left the world before its passenger was ashore")
	}
	late.Send(encodeMoveInVehicle(old, deckSpot, deckCenter))
	late.Send(encodeGetOnVehicle(old, deckSpot))
	srv.AwaitHandled(t)
	letGo()
	srv.Settle(t)

	newBoats := srv.Boats.Boats()
	if len(newBoats) != 1 {
		t.Fatalf("fleet after the reload = %d boats, want 1", len(newBoats))
	}
	fresh := newBoats[0].ObjectID()
	x, y, z := oldBoat.Position()
	stoppedAt := location.Location{X: x, Y: y, Z: z}
	// The deck walk shows as on any boat in sight; the boarding is refused
	// with no GetOnVehicle and no peace zone entered.
	assertLog(t, "late boarder", reloadLog(t, srv, late),
		started(old, 0), info(old, stoppedAt, oldBoat.Heading()),
		deckWalk(lateID, old, deckSpot, deckCenter), af, af,
		teleport(riderID, reloadTown), deleted(riderID),
		deleted(old), info(fresh, runeDock, 1000))
	if _, ok := srv.State.Object(old); ok {
		t.Fatalf("boat %d still in the world", old)
	}
	if got := oldBoat.Passengers(); len(got) != 0 {
		t.Fatalf("the removed boat still carries %v", got)
	}
	if x, y, _ := srv.PlayerPosition(t, riderID); x != reloadTown.X || y != reloadTown.Y {
		t.Fatalf("passenger at %d,%d, want ashore at %v", x, y, reloadTown)
	}

	// Riding nothing, the player boards the new boat.
	late.Send(encodeMoveInVehicle(fresh, deckSpot, deckCenter))
	late.Send(encodeGetOnVehicle(fresh, deckSpot))
	assertLog(t, "boarding the new boat", passengerLog(t, srv, late),
		deckWalk(lateID, fresh, deckSpot, deckCenter), af,
		sm(serverpackets.SystemMessageEnterPeacefulZone), getOn(lateID, fresh, deckSpot))
	if !slices.Contains(newBoats[0].Passengers(), lateID) {
		t.Fatalf("new boat passengers = %v, want %d aboard", newBoats[0].Passengers(), lateID)
	}
}
