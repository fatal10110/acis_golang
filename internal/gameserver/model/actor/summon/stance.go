package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// Running reports whether a is in its run stance. A summon runs from the
// moment it is called out; a hungry pet walks.
func (a *Actor) Running() bool { return !a.walking.Load() }

// forceRunStance puts a walking summon in its run stance.
func (a *Actor) forceRunStance() { a.setStance(true) }

// forceWalkStance puts a running summon in its walk stance.
func (a *Actor) forceWalkStance() { a.setStance(false) }

// setStance switches a to its run stance when run is set, to its walk stance
// otherwise, and does nothing when it already stands so. The movement takes
// the new stance's speed, observers see the stance change unless the summon
// cannot move at all, and its status is broadcast again.
func (a *Actor) setStance(run bool) {
	if !a.walking.CompareAndSwap(run, !run) {
		return
	}
	a.refreshMoveSpeed()
	if a.MoveSpeed() != 0 {
		a.emit(event.MoveTypeChanged{Running: run})
	}
	a.BroadcastStatus()
}
