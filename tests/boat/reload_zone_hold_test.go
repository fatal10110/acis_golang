package boat

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestReloadZoneKeepsBoatPeace: //reload zone leaves a passenger in the peace
// its boat keeps it in, with summoning it still barred: the boat raised
// those, not a zone the reload replaced.
func TestReloadZoneKeepsBoatPeace(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, me := bootReloadWith(t, nil, &fail, []route.BoatItinerary{passengerItinerary()},
		gameservertest.WithZones(zone.NewIndex()),
		gameservertest.WithDataReloads(network.DataReloads{Zones: func() (*zone.Index, error) { return zone.NewIndex(), nil }}),
	)
	board(t, srv, gm, srv.Boats.Boats()[0].ObjectID())
	obj, ok := srv.State.Player(me)
	if !ok {
		t.Fatal("passenger missing from world state")
	}
	passenger, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("passenger has no live character")
	}
	if !passenger.InPeaceZone() || !passenger.NoSummonFriendZone() {
		t.Fatalf("aboard: peace %v, no summon friend %v; want both", passenger.InPeaceZone(), passenger.NoSummonFriendZone())
	}

	gm.Send(encodeBuildCmd("reload zone"))
	if got := reloadLog(t, srv, gm); len(got) == 0 || got[len(got)-1] != "text Zones have been reloaded." {
		t.Fatalf("//reload zone answered %q, want the reload's answer last", got)
	}
	if !passenger.InPeaceZone() || !passenger.NoSummonFriendZone() {
		t.Fatalf("after //reload zone: peace %v, no summon friend %v; want both", passenger.InPeaceZone(), passenger.NoSummonFriendZone())
	}
}
