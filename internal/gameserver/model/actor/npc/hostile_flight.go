package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// FlyTo broadcasts a forced-flight animation without changing server position.
func (h *Hostile) FlyTo(dest location.Location, flight modelskill.Flight) {
	h.emit(event.Flight{Dest: dest, Flight: flight})
}

// TeleportTo jumps the NPC to target. It aborts movement, attack and cast,
// grounds target unless it lies in water, announces the jump to the NPC's
// current observers, then leaves and re-enters the world grid so every
// observer around either end forgets and rediscovers it; the NPC in turn
// drops the threat of every creature around its old position, and every
// hostile around that position drops the NPC's (DropThreatAround). A
// teleport that starts while another is in progress is dropped. A teleport
// clears the geo-path fail streak: the next pathfinding attempt is from a
// new cell, not a continuation of the stall that triggered recovery.
func (h *Hostile) TeleportTo(target location.Location) {
	h.TeleportJump(location.Jump{To: target})
}

// TeleportJump is TeleportTo aimed where j lands: a scattered jump stops at
// the last point the geodata lets a straight walk from j.To toward
// j.Toward reach, before the landing is grounded.
func (h *Hostile) TeleportJump(j location.Jump) {
	target := j.To
	if h.Live != nil {
		if !h.SetTeleporting(true) {
			return
		}
		target = move.Land(h.Move(), j, h.inWater)
	}
	h.brain.AbortAll()
	h.emit(event.Teleported{To: target})
	// The NPC leaves the zones around its old position while its observers
	// there still know it, and enters those at target once it has landed.
	h.zones.leave()
	// Leaving the grid forgets every creature around the old position, and
	// its threat entry (damage and hate) goes with it even when the
	// destination still sees it. Queued attack desires stay; the next
	// combat-memory refresh handles those.
	threats := h.brain.Threats()
	for _, threat := range threats.Snapshot() {
		if h.Knows(threat.Attacker) {
			threats.Remove(threat.Attacker)
		}
	}
	DropThreatAround(h.world, h)
	if h.Live != nil {
		h.Move().SetPosition(target)
	}
	if h.world != nil {
		_ = h.world.Teleport(h, target.X, target.Y, target.Z)
	}
	h.SetTeleporting(false)
	h.zones.enter()
	h.ResetGeoPathFailCount()
}

// DropThreatAround removes subject's threat entry (damage and hate) from
// every hostile NPC in the region neighborhood around subject's current
// position. A teleport calls it before subject leaves that position: every
// hostile around it forgets subject, even one that still sees it after the
// jump. Spell hate and queued attack desires are left alone; the next
// combat-memory refresh drops attack desires whose threat is gone.
func DropThreatAround(w *world.State, subject interface {
	world.Tracked
	attackable.Combatant
},
) {
	if w == nil {
		return
	}
	w.ForEachKnown(subject, func(o world.Tracked) {
		if h, ok := o.(*Hostile); ok {
			h.brain.Threats().Remove(subject)
		}
	})
}

// SetWaterZone installs the query TeleportTo uses to keep a destination
// inside a water zone at its own height. It must be set before the NPC is
// published.
func (h *Hostile) SetWaterZone(inWater func(location.Location) bool) {
	h.inWater = inWater
}

// SetXYZ moves the NPC immediately and reseeds its ordinary movement state.
func (h *Hostile) SetXYZ(x, y, z int) {
	position := location.Location{X: x, Y: y, Z: z}
	if h.Live != nil {
		h.Move().SetPosition(position)
	}
	h.relocate(position, true)
}

// BroadcastPosition sends the forced-location correction after a flight lands.
func (h *Hostile) BroadcastPosition() {
	h.emit(event.PositionCorrected{})
}
