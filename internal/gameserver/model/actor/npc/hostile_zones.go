package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// SetZones makes ix the zones the NPC's membership follows. It must be set
// before the NPC is published; without it the NPC stands in no zone.
func (h *Hostile) SetZones(ix *zone.Index) {
	h.zones.ix = ix
}

// EnterZones enters the NPC into the zones at its position. Call it once
// the NPC is spawned into the world.
func (h *Hostile) EnterZones() { h.zones.enter() }

// SettleZones revalidates the NPC's zones at once, as a move ends.
func (h *Hostile) SettleZones() { h.zones.settle() }

// InsideZone reports whether the NPC's zones hold flag.
func (h *Hostile) InsideZone(flag zone.Flag) bool { return h.zones.has(flag) }

func (h *Hostile) zoneTeleporting() bool { return h.Teleporting() }

// swimStateChanged switches the NPC's movement into or out of swimming and
// shows its observers its new state: the stationary view for an NPC that
// cannot move at its current speed, the full one otherwise.
func (h *Hostile) swimStateChanged(swimming bool) {
	if h.Live != nil {
		h.Move().SetSwimming(swimming)
	}
	h.emit(event.NPCInfoChanged{ServerObject: h.MoveSpeed() == 0})
}
