package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Running reports whether this NPC is in run rather than walk stance.
func (h *Hostile) Running() bool {
	return h.running.Load()
}

// SetRunning updates run/walk stance and reports whether it changed. A
// change re-times the movement simulation to the new stance's speed.
func (h *Hostile) SetRunning(running bool) bool {
	for {
		current := h.running.Load()
		if current == running {
			return false
		}
		if h.running.CompareAndSwap(current, running) {
			h.refreshMoveSpeed()
			return true
		}
	}
}

// ForceWalkStance switches to walk stance and broadcasts when the stance
// changes and this NPC can move.
func (h *Hostile) ForceWalkStance() {
	if !h.Running() {
		return
	}
	h.setWalkOrRun(false)
}

// ForceRunStance switches to run stance and broadcasts when the stance
// changes and this NPC can move.
func (h *Hostile) ForceRunStance() {
	if h.Running() {
		return
	}
	h.setWalkOrRun(true)
}

func (h *Hostile) setWalkOrRun(running bool) {
	if !h.SetRunning(running) {
		return
	}
	if h.MoveSpeed() == 0 {
		return
	}
	h.emit(event.MoveTypeChanged{Running: h.Running()})
}

// MoveSpeed is this NPC's current move speed: the template run or walk
// speed, whichever its stance picks, through the RUN_SPEED stat, narrowed
// to float32 like the client-facing speed.
func (h *Hostile) MoveSpeed() float64 {
	base := int(h.Instance.Template.WalkSpeed)
	if h.Running() {
		base = int(h.Instance.Template.RunSpeed)
	}
	return float64(float32(h.calcStat(stat.RunSpeed, float64(base))))
}

// refreshMoveSpeed hands the current move speed to the movement
// simulation, re-timing a leg in flight, so the server keeps pace with the
// speed the client animates.
func (h *Hostile) refreshMoveSpeed() {
	if h.Live == nil {
		return
	}
	h.Move().SetSpeed(h.MoveSpeed())
}
