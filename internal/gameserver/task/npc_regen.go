package task

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// NPCRegenTick is the fixed HP/MP regeneration period (Formulas.
// getRegeneratePeriod's HP_REGENERATE_PERIOD, 3s).
const NPCRegenTick = 3 * time.Second

type npcRegenActor interface {
	TickRegen()
}

// NPCRegen runs periodic HP/MP regeneration for spawned attackable NPCs.
type NPCRegen struct {
	state *world.State
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
	for _, obj := range r.state.Objects() {
		if actor, ok := obj.(npcRegenActor); ok {
			actor.TickRegen()
		}
	}
}
