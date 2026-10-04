package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Level returns this NPC's template level for skill and reward formulas.
func (h *Hostile) Level() int {
	return h.Instance.Template.Level
}

// SpoilPool returns this NPC life's spoil state.
func (h *Hostile) SpoilPool() *item.SpoilPool {
	return &h.spoil
}

// Spoiled reports whether this NPC has been marked by a spoil skill.
func (h *Hostile) Spoiled() bool {
	return h.spoil.IsSpoiled()
}

// SpoilerID returns the player that marked this NPC for spoil, or 0.
func (h *Hostile) SpoilerID() int32 {
	return h.spoil.SpoilerID()
}

// SeedState returns this NPC life's manor seed state.
func (h *Hostile) SeedState() *SeedState {
	return &h.seed
}

// Seeded reports whether this NPC has been sown for manor harvest.
func (h *Hostile) Seeded() bool {
	return h.seed.Seeded()
}

// HasCorpse reports whether this dead NPC still exposes a corpse to target
// handlers.
func (h *Hostile) HasCorpse() bool {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	return h.dead && !h.decayed && !h.corpseDeadline.IsZero()
}

// SetCorpseDeadline records the same deadline registered with the decay
// task.
func (h *Hostile) SetCorpseDeadline(deadline time.Time) {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	h.corpseDeadline = deadline
}

// CorpseDeadline returns this corpse's decay deadline, if one is active.
func (h *Hostile) CorpseDeadline() (time.Time, bool) {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	return h.corpseDeadline, !h.corpseDeadline.IsZero()
}

// CorpseTime returns this NPC template's normal corpse display duration.
func (h *Hostile) CorpseTime() time.Duration {
	return time.Duration(h.Instance.Template.CorpseTime) * time.Second
}
