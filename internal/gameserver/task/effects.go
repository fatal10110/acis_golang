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
// least one buff or debuff. Lists register with their explicit activity
// registry when their first effect lands and deregister when they drain.
//
// SetActive is safe to call concurrently with Tick; Tick only ever runs
// on the scheduler ticker's single goroutine, one call at a time.
type Effects struct {
	log zerolog.Logger

	*activeRegistry[*effect.List, *effect.List]
}

// NewEffects returns an empty active-effect-list registry.
func NewEffects() *Effects {
	e := &Effects{activeRegistry: newActiveRegistry[*effect.List, *effect.List]()}
	return e
}

// SetActive registers or deregisters list depending on whether it just
// gained or lost its last effect.
func (e *Effects) SetActive(list *effect.List, active bool) {
	if active {
		e.add(list, list)
	} else {
		e.remove(list)
	}
}

// Reset deregisters every currently registered list, so Tick visits
// nothing until something registers again. It is server-teardown cleanup
// for a list left registered without a clean despawn, logout, or Untrack;
// each reset list is untracked for good, like its owner leaving the world.
//
// It goes through each list's own Untrack rather than clearing e.entries
// directly, so List.tracked stays in sync with the registry. The snapshot
// is taken and released before calling Untrack, not held across the calls:
// Untrack ends in List.activity.SetActive -> Effects.SetActive -> e.remove,
// which needs e.mu itself, so holding e.mu here would deadlock.
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

// Tick advances every currently active effect list once, on its owner's
// queue.
func (e *Effects) Tick() {
	if !e.beginTick(e.log, "task: Effects.Tick") {
		return
	}
	defer e.endTick()
	defer e.releaseSnapshot()

	for _, list := range e.snapshot() {
		list.Queue().Post(list.Tick)
	}
}
