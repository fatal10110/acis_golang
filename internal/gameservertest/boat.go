package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// WithBoats spawns one boat per itinerary at boot through the production
// fleet, with the production boat sinks. The fleet's ticker is never
// started: a suite drives the boats with Server.Boats.Tick, one position
// update (a tenth of a second) per call.
func WithBoats(itineraries ...route.BoatItinerary) Option {
	return func(o *options) { o.boats = append(o.boats, itineraries...) }
}

func bootBoats(t *testing.T, itineraries []route.BoatItinerary, ids *sequentialIDs, state *world.State) *boat.Fleet {
	t.Helper()
	if len(itineraries) == 0 {
		return nil
	}
	fleet, err := boat.New(itineraries, ids, state, network.BoatSinks(state))
	if err != nil {
		t.Fatalf("boat fleet: %v", err)
	}
	return fleet
}
