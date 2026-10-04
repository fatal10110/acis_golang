package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// agitMapTable makes every tile Giran's restart region (map 918), while a
// restart area over the whole world sends every race to Gludio (map 912):
// only the plain region point names Giran.
func agitMapTable(t *testing.T) *restart.Table {
	t.Helper()
	var regions []location.Point
	for x := geo.TileXMin; x <= geo.TileXMax; x++ {
		for y := geo.TileYMin; y <= geo.TileYMax; y++ {
			regions = append(regions, location.Point{X: x, Y: y})
		}
	}
	everyRace := map[player.Race]string{}
	for r := player.RaceHuman; r <= player.RaceDwarf; r++ {
		everyRace[r] = "Gludio"
	}
	area, err := restart.NewArea([]location.Point{
		{X: geo.WorldXMin, Y: geo.WorldYMin},
		{X: geo.WorldXMax, Y: geo.WorldYMin},
		{X: geo.WorldXMax, Y: geo.WorldYMax},
		{X: geo.WorldXMin, Y: geo.WorldYMax},
	}, -100000, 100000, everyRace)
	if err != nil {
		t.Fatal(err)
	}
	town := location.Location{X: 20000, Y: 20000, Z: gameservertest.SpawnZ}
	return &restart.Table{
		Areas: []restart.Area{area},
		Points: []restart.Point{
			{Name: "Giran", LocName: 918, Points: []location.Location{town}, ChaoPoints: []location.Location{town}, MapRegions: regions},
			{Name: "Gludio", LocName: 912, Points: []location.Location{town}, ChaoPoints: []location.Location{town}},
		},
	}
}

// TestAuctioneerLocationMap: the location page is the town map of the
// restart point whose map regions cover the player's tile, with the back
// link to the chat window; no restart area or race ban steers it.
func TestAuctioneerLocationMap(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{halls: []hallRow{{id: moonstone, endDate: end}}},
		gameservertest.WithRestartPoints(agitMapTable(t)))
	oid := w.oid()
	w.open(t)
	page := w.page(t, "location")
	if want := fill(auctionPage(t, "map_agit_giran.htm"), "%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_start"); page != want {
		t.Fatalf("location page =\n%s\nwant\n%s", page, want)
	}
}
