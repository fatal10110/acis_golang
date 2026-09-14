package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// FlyTo broadcasts a forced-flight animation without changing server position.
func (h *Hostile) FlyTo(dest location.Location, flight modelskill.Flight) {
	h.emit(event.Flight{Dest: dest, Flight: flight})
}

// TeleportTo snaps the NPC to target and broadcasts the forced correction.
// A teleport clears the geo-path fail streak: the next pathfinding attempt
// is from a new cell, not a continuation of the stall that triggered recovery.
func (h *Hostile) TeleportTo(target location.Location) {
	h.SetXYZ(target.X, target.Y, target.Z)
	h.BroadcastPosition()
	h.ResetGeoPathFailCount()
}

// SetXYZ moves the NPC immediately and reseeds its ordinary movement state.
func (h *Hostile) SetXYZ(x, y, z int) {
	position := location.Location{X: x, Y: y, Z: z}
	if h.Live != nil {
		h.Move().SetPosition(position)
	}
	h.SyncPosition(position)
}

// BroadcastPosition sends the forced-location correction after a flight lands.
func (h *Hostile) BroadcastPosition() {
	h.emit(event.PositionCorrected{})
}
