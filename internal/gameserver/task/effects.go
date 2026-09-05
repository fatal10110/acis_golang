package task

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// EffectTick is the fixed live-effect sweep interval.
const EffectTick = time.Second

// Effects runs periodic actions for effect lists that currently hold at
// least one buff or debuff. Lists register themselves through
// effect.SetActivityHook (wired to trackActivity by NewEffects) the moment
// their first effect lands, and deregister the moment they drain back to
// empty, so Tick never scans lists with nothing to do.
//
// trackActivity is safe to call concurrently with Tick; Tick only ever runs
// on the scheduler ticker's single goroutine, one call at a time.
type Effects struct {
	log zerolog.Logger

	*activeRegistry[*effect.List, *effect.List]
}

// NewEffects returns an empty active-effect-list registry and installs it as
// the process-wide effect.List activity registrar. Call it once, before any
// effect.List is constructed.
func NewEffects() *Effects {
	e := &Effects{activeRegistry: newActiveRegistry[*effect.List, *effect.List]()}
	effect.SetActivityHook(e.trackActivity)
	return e
}

// trackActivity registers or deregisters list depending on whether it just
// gained or lost its last effect.
func (e *Effects) trackActivity(list *effect.List, active bool) {
	if active {
		e.add(list, list)
	} else {
		e.remove(list)
	}
}

// Start launches the fixed live-effect task.
func (e *Effects) Start(log zerolog.Logger) *scheduler.Ticker {
	e.log = log
	return scheduler.Start(EffectTick, e.Tick, log)
}

// Tick advances every currently active effect list once.
func (e *Effects) Tick() {
	if !e.beginTick(e.log, "task: Effects.Tick") {
		return
	}
	defer e.endTick()
	defer e.releaseSnapshot()

	for _, list := range e.snapshot() {
		list.Tick()
	}
}
