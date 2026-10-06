package ai

// HookPoint names a place in an NPC's think pass, or in its arrival, where
// the behavior bound to its template takes its turn. The AI loop releases
// its lock for the call and takes it again after: the behavior may raise a
// think, on its own goroutine or another, and that think runs at once
// instead of waiting for the pass. Whatever the pass reads after the point
// it reads afresh.
type HookPoint uint8

const (
	// HookNoDesire ends the idle: after the abort, the walk stance and the
	// switch to idle (which the periodic cycle's idle skips while desire
	// selection is held), before the idle follow or wander is queued.
	HookNoDesire HookPoint = iota + 1
	// HookSeeCreature opens the periodic cycle, before invalid desires are
	// dropped and a desire is chosen.
	HookSeeCreature
	// HookMoveFinished is a walk ending at its destination, on arrival or on
	// a walk step that finds the actor already there, before the walk's
	// desire is dropped.
	HookMoveFinished
	// HookOutOfTerritory is the first arrival outside territory, before the
	// stale-hate sweep is armed.
	HookOutOfTerritory
)

// atHookPoint hands the actor hook point p with mu released. The caller
// holds mu and holds it again on return; a think the point raises, or one
// another goroutine runs meanwhile, may have changed anything mu guards.
func (a *Attackable) atHookPoint(p HookPoint) {
	a.mu.Unlock()
	defer a.mu.Lock()
	a.actor.AtHookPoint(p)
}
