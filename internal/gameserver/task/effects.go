package task

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// EffectTick is the fixed live-effect sweep interval.
const EffectTick = time.Second

type effectOwner interface {
	EffectList() *effect.List
}

// Effects runs periodic actions for live actors' active effects.
type Effects struct {
	state *world.State
}

// NewEffects returns a live-effect ticker over state's spawned actors.
func NewEffects(state *world.State) *Effects {
	return &Effects{state: state}
}

// Start launches the fixed live-effect task.
func (e *Effects) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(EffectTick, e.Tick, log)
}

// Tick advances every spawned actor's effect list once.
func (e *Effects) Tick() {
	if e == nil || e.state == nil {
		return
	}
	for _, obj := range e.state.Objects() {
		owner, ok := obj.(effectOwner)
		if !ok {
			continue
		}
		if list := owner.EffectList(); list != nil {
			list.Tick()
		}
	}
}
