package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/world"

// Hide takes this NPC out of the world as the reference's decayMe hides
// one: it leaves its zones and the players around it see it go. Unlike a
// decay, no decayed hook runs and nothing respawns it. The hidden NPC takes
// no more work: its effects end, its queue closes and its behavior timers
// stop, with no hook, where the reference leaves a hidden NPC's AI and
// timers running with nothing left to see them. It reports whether this
// call hid it; an NPC already decayed or hidden is left as it is.
func (h *Hostile) Hide(worldState *world.State) bool {
	if !h.markDecayed() {
		return false
	}
	if h.scripts != nil {
		h.scripts.HostileHidden(h)
	}
	h.leaveWorld(worldState, nil)
	return true
}
