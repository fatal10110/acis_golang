package boat

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// dockSite is where a boat ties up at a dock. An exclusive dock holds one
// boat at a time: a boat bound for it waits off shore while another is tied
// up there, and every exclusive dock starts the server held.
type dockSite struct {
	at        location.Location
	exclusive bool
}

var dockSites = map[route.Dock]dockSite{
	route.DockTalkingIsland: {at: location.Location{X: -96622, Y: 261660, Z: -3610}, exclusive: true},
	route.DockGludin:        {at: location.Location{X: -95686, Y: 150514, Z: -3610}, exclusive: true},
	route.DockRune:          {at: location.Location{X: 34381, Y: -37680, Z: -3610}, exclusive: true},
	route.DockGiran:         {at: location.Location{X: 48950, Y: 190613, Z: -3610}},
	route.DockPrimeval:      {at: location.Location{X: 10342, Y: -27279, Z: -3610}},
	route.DockInnadril:      {at: location.Location{X: 111384, Y: 226232, Z: -3610}},
}

// docks tracks which exclusive docks a boat holds. Docks are shared between
// itineraries (Gludin and Rune each serve two), so one docks value serves a
// whole fleet. It is owned by the fleet's tick goroutine.
type docks struct {
	held map[route.Dock]bool
}

func newDocks() *docks { return &docks{held: make(map[route.Dock]bool)} }

// busy reports whether d is an exclusive dock a boat holds.
func (s *docks) busy(d route.Dock) bool {
	return dockSites[d].exclusive && s.held[d]
}

// setBusy marks d held or free. A dock that is not exclusive never counts as
// held.
func (s *docks) setBusy(d route.Dock, held bool) {
	if dockSites[d].exclusive {
		s.held[d] = held
	}
}
