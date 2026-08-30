package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Running reports whether this NPC is in run rather than walk stance.
func (h *Hostile) Running() bool {
	return h.running.Load()
}

// SetRunning updates run/walk stance and reports whether it changed.
func (h *Hostile) SetRunning(running bool) bool {
	for {
		current := h.running.Load()
		if current == running {
			return false
		}
		if h.running.CompareAndSwap(current, running) {
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
	if h.moveSpeed() == 0 {
		return
	}
	if h.frames == nil {
		return
	}
	_ = h.broadcastFrame(func() wire.Frame {
		return h.frames.ChangeMoveType(h.ObjectID(), h.Running())
	})
}

func (h *Hostile) moveSpeed() int {
	if h.Running() {
		return h.RunSpeed()
	}
	return int(h.Instance.Template.WalkSpeed)
}
