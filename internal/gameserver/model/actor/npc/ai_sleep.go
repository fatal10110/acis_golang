package npc

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// aiSleep is an NPC's AI-sleep latch: whether the periodic AI task leaves
// it alone. It is decided only at the NPC's lifecycle points — coming onto
// the grid, its region waking or going quiet, a teleport landing it outside
// its territory, its death and its removal — and holds until the next one,
// wherever the NPC walks in between. The zero latch is asleep. Safe from
// any goroutine.
type aiSleep struct{ awake atomic.Bool }

// aiSleeper is the NPC an aiSleep belongs to.
type aiSleeper interface {
	world.Tracked
	Dead() bool
	InTerritory() bool
	noSleepMode() bool
}

// settle decides the latch from where npc stands now: awake while it is
// alive on the grid in an active region, or in a quiet one it does not
// sleep in — a noSleepMode NPC or one outside its territory. Off the world
// (w nil) a live NPC is awake. The region and death are read again after
// the store, so a region toggle or death that landed meanwhile, whose own
// settle may have stored first, cannot be overwritten with this call's
// older answer.
func (l *aiSleep) settle(w *world.State, npc aiSleeper) {
	for {
		placed, active := regionActivityOf(w, npc)
		dead := npc.Dead()
		l.awake.Store(placed && !dead && (active || npc.noSleepMode() || !npc.InTerritory()))
		if p, a := regionActivityOf(w, npc); p == placed && a == active && npc.Dead() == dead {
			return
		}
	}
}

// regionActivityOf is w.RegionActivity, with everything placed and active
// when there is no world.
func regionActivityOf(w *world.State, t world.Tracked) (placed, active bool) {
	if w == nil {
		return true, true
	}
	return w.RegionActivity(t)
}

// wakeOutOfTerritory wakes the latch of a live npc that a teleport landed
// outside its territory; one landing inside keeps the latch as it was.
func (l *aiSleep) wakeOutOfTerritory(npc aiSleeper) {
	if npc.Dead() || npc.InTerritory() {
		return
	}
	l.awake.Store(true)
	// A death that landed meanwhile has already stored asleep; keep it.
	if npc.Dead() {
		l.awake.Store(false)
	}
}

// sleep puts the latch to sleep, at the NPC's death or removal.
func (l *aiSleep) sleep() { l.awake.Store(false) }

// asleep reports the latch.
func (l *aiSleep) asleep() bool { return !l.awake.Load() }
