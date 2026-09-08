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

// Reset deregisters every currently registered list, so Tick visits
// nothing until something registers again. Production never calls this —
// it is the mop-up gameservertest runs between test servers that share one
// Effects instance per process (see NewEffects), for whatever a test left
// registered without a clean despawn/logout/Untrack (a spawned NPC or
// EffectPoint the test never killed, decayed, or explicitly tore down).
//
// It goes through each list's own Untrack rather than clearing e.entries
// directly: a list tracks its own registration state (List.tracked) so
// notifyActivityTransition can decide+apply atomically, and a bare
// clear(e.entries) would leave that state out of sync for any list that
// survives the reset — the list would believe itself still registered and
// silently refuse to re-register on its next Add. Untrack keeps the two in
// sync by clearing List.tracked itself. The snapshot is taken and released
// before calling Untrack, not held across the calls: Untrack ends in
// callActivityHook -> trackActivity -> e.remove, which needs e.mu itself,
// so calling it while still holding e.mu here would deadlock.
func (e *Effects) Reset() {
	e.mu.Lock()
	lists := make([]*effect.List, 0, len(e.entries))
	for list := range e.entries {
		lists = append(lists, list)
	}
	e.mu.Unlock()

	for _, list := range lists {
		list.Untrack()
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
