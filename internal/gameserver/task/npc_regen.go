package task

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// NPCRegenTick is the fixed HP/MP regeneration period (Formulas.
// getRegeneratePeriod's HP_REGENERATE_PERIOD, 3s).
const NPCRegenTick = 3 * time.Second

type npcRegenActor interface {
	TickRegen()
}

// NPCRegen runs periodic HP/MP regeneration for spawned attackable NPCs.
//
// scratch is a per-tick scan buffer reused across calls instead of
// reallocated: Tick only ever runs on the scheduler ticker's single
// goroutine, so nothing else touches it between ticks.
type NPCRegen struct {
	state   *world.State
	scratch []worldobject.Object
}

// NewNPCRegen returns an NPC regen ticker over state's spawned actors.
func NewNPCRegen(state *world.State) *NPCRegen {
	return &NPCRegen{state: state}
}

// Start launches the fixed NPC regen task.
func (r *NPCRegen) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(NPCRegenTick, r.Tick, log)
}

// Tick advances every spawned attackable NPC's HP/MP regeneration once.
func (r *NPCRegen) Tick() {
	if r == nil || r.state == nil {
		return
	}
	r.scratch = r.state.AppendObjects(r.scratch)
	for _, obj := range r.scratch {
		if actor, ok := obj.(npcRegenActor); ok {
			actor.TickRegen()
		}
	}
	// Drop references past this tick's length so a shrinking population
	// doesn't keep despawned objects reachable through unused capacity.
	clear(r.scratch[:cap(r.scratch)])
	r.scratch = r.scratch[:0]
}
