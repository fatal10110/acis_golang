package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"

// deathIdle is the idle a death leaves the NPC in, run by Die once
// observers saw it die: its walk, swing and cast are already stopped, so it
// switches to its walk stance and opens the no-desire point, once. Die runs
// it with the AI loop unlocked, as every hook point is.
func (h *Hostile) deathIdle() {
	h.ForceWalkStance()
	h.AtHookPoint(ai.HookNoDesire)
}
