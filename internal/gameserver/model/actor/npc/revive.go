package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// Revive stands this dead NPC up again with restoreHP of its maximum HP,
// counted in whole points; its MP stays as it is. The players targeting it
// are offered the restored HP, which starts its regeneration, then every
// player that knows it sees it rise. Its AI stays asleep until something
// wakes it, as after its death. A living NPC, or a corpse that has decayed,
// is left as it is, and Revive reports false.
//
// The caller drops the corpse's pending decay first: the decay task would
// otherwise take the revived NPC out of the world at the old deadline.
func (h *Hostile) Revive(restoreHP float64) bool {
	maxHP := h.MaxHPValue()
	h.deathMu.Lock()
	if !h.dead || h.decayed {
		h.deathMu.Unlock()
		return false
	}
	h.dead = false
	h.corpseDeadline = time.Time{}
	// HP is set while the dead flag is cleared, so no hit lands on a living
	// NPC at 0 HP.
	h.health.Revive(min(maxHP*restoreHP, maxHP))
	h.deathMu.Unlock()
	h.BroadcastStatus()
	h.emit(event.Revived{})
	return true
}

// Revive stands this dead NPC up again as Hostile.Revive does.
func (f *Folk) Revive(restoreHP float64) bool {
	maxHP := f.MaxHPValue()
	f.vitalsMu.Lock()
	if !f.dead || f.decayed {
		f.vitalsMu.Unlock()
		return false
	}
	f.dead = false
	f.corpseDeadline = time.Time{}
	f.hp = min(maxHP*restoreHP, maxHP)
	f.vitalsMu.Unlock()
	f.BroadcastStatus()
	f.emit(event.Revived{})
	return true
}
