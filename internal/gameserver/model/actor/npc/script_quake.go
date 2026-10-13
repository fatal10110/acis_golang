package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ShakeScreens shakes the screens of the players around h, centred where h
// stands now, with intensity, for duration seconds.
func (h *Hostile) ShakeScreens(intensity, duration int32) {
	x, y, z := h.Position()
	h.emit(event.EarthquakeStarted{At: location.Location{X: x, Y: y, Z: z}, Intensity: intensity, Duration: duration})
}
