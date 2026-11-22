package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Level returns this NPC's template level for skill and reward formulas.
func (h *Hostile) Level() int {
	return h.Instance.Template.Level
}

// lifeLoot is what one life of a hostile NPC keeps of the skills that
// change its rewards: the spoil mark and pool, the manor seed and an armed
// over-hit. Most lives see none of them, so it is made on the first write
// and a life without one reads as unspoiled, unsown and not over-hit.
type lifeLoot struct {
	spoil   item.SpoilPool
	seed    SeedState
	overhit overhitState
}

// ensureLoot returns this life's loot state, making it on first use.
// Attackers write it from their own queues, so the first one published
// is the one every caller gets. A respawn clears it in place: it is never
// replaced.
func (h *Hostile) ensureLoot() *lifeLoot {
	if l := h.loot.Load(); l != nil {
		return l
	}
	l := &lifeLoot{}
	l.seed.ownerLevel, l.seed.cropBase = seedOwnerData(h.Instance.Template)
	if h.loot.CompareAndSwap(nil, l) {
		return l
	}
	return h.loot.Load()
}

// SpoilPool returns this NPC life's spoil state, made on first use; for a
// read that should not make it, see ExistingSpoilPool.
func (h *Hostile) SpoilPool() *item.SpoilPool {
	return &h.ensureLoot().spoil
}

// ExistingSpoilPool returns this NPC life's spoil state, nil when no spoil
// or sweep skill has touched it.
func (h *Hostile) ExistingSpoilPool() *item.SpoilPool {
	if l := h.loot.Load(); l != nil {
		return &l.spoil
	}
	return nil
}

// Spoiled reports whether this NPC has been marked by a spoil skill.
func (h *Hostile) Spoiled() bool {
	l := h.loot.Load()
	return l != nil && l.spoil.IsSpoiled()
}

// SpoilerID returns the player that marked this NPC for spoil, or 0.
func (h *Hostile) SpoilerID() int32 {
	if l := h.loot.Load(); l != nil {
		return l.spoil.SpoilerID()
	}
	return 0
}

// Sweepable reports whether this NPC's spoil pool holds anything left to
// sweep.
func (h *Hostile) Sweepable() bool {
	l := h.loot.Load()
	return l != nil && l.spoil.Sweepable()
}

// SeedState returns this NPC life's manor seed state, made on first use.
func (h *Hostile) SeedState() *SeedState {
	return &h.ensureLoot().seed
}

// Seeded reports whether this NPC has been sown for manor harvest.
func (h *Hostile) Seeded() bool {
	l := h.loot.Load()
	return l != nil && l.seed.Seeded()
}

// SeedBlocksDrops reports whether this life's seed suppresses its ordinary
// item drops (see SeedState.BlocksDrops); false for an unsown life.
func (h *Hostile) SeedBlocksDrops() bool {
	l := h.loot.Load()
	return l != nil && l.seed.BlocksDrops()
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

// scheduleDecay registers this corpse with the decay task for its template
// corpse time, twice that for a spoiled or sown corpse, so it lasts for the
// sweep or the harvest. A template with no corpse time leaves the corpse.
func (h *Hostile) scheduleDecay() {
	if h.decay == nil || h.Instance.Template.CorpseTime <= 0 {
		return
	}
	interval := h.CorpseTime()
	if h.Spoiled() || h.Seeded() {
		interval *= 2
	}
	h.SetCorpseDeadline(h.decay.Add(h, interval))
}
