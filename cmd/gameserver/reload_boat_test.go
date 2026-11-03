package main

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// TestDataReloadsReadBoatRoutes requires the boat reload hook to read the
// datapack's boatRoutes.xml anew, and to fail on a file it cannot read.
func TestDataReloadsReadBoatRoutes(t *testing.T) {
	root := t.TempDir()
	writeDataFile(t, root, "", "boatRoutes.xml", `<?xml version="1.0" encoding="utf-8"?>
<list>
	<itinerary dock1="RUNE" dock2="PRIMEVAL" item1="8925" item2="8924" heading="40785">
		<route>
			<node x="33381" y="-37680" z="-3610" speed="200" rotation="800" />
			<node x="32381" y="-37680" z="-3610" speed="200" rotation="800" busy="1994" />
		</route>
		<route>
			<node x="33381" y="-36680" z="-3610" speed="180" rotation="800" />
			<node x="34381" y="-37680" z="-3610" speed="220" rotation="800" busy="1993" />
		</route>
	</itinerary>
</list>`)
	reloads := provideDataReloads(gameServerPaths{DataRoot: root}, gameServerConfig{}, &gameData{}, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	itineraries, err := reloads.BoatRoutes()
	if err != nil {
		t.Fatalf("boat reload: %v", err)
	}
	if len(itineraries) != 1 || itineraries[0].Heading != 40785 || len(itineraries[0].Routes) != 2 ||
		itineraries[0].Routes[0].Dock != route.DockRune || itineraries[0].Routes[1].ItemID != 8924 {
		t.Fatalf("reloaded itineraries = %+v, want the Rune-Primeval round trip", itineraries)
	}

	writeDataFile(t, root, "", "boatRoutes.xml", "<list><itinerary")
	if _, err := reloads.BoatRoutes(); err == nil {
		t.Fatal("boat reload of a broken file succeeded")
	}
}
