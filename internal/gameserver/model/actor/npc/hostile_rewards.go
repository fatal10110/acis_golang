package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
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

// SeedState is one hostile life's manor seed lifecycle.
type SeedState struct {
	sowerID   int32
	seed      manor.Seed
	harvested bool
}

// Seeded reports whether this hostile was sown during its current life.
func (s *SeedState) Seeded() bool {
	return s.sowerID != 0
}

// Sow records the sower and seed carried by this hostile.
func (s *SeedState) Sow(sowerID int32, seed manor.Seed) {
	s.sowerID = sowerID
	s.seed = seed
	s.harvested = false
}

// Harvested reports whether this seeded hostile was already harvested.
func (s *SeedState) Harvested() bool {
	return s.harvested
}

// MarkHarvested marks this seeded hostile's crop as consumed.
func (s *SeedState) MarkHarvested() {
	s.harvested = true
}

// AllowedToHarvest currently permits only the original sower. Party sharing
// is deferred until live party membership is available.
func (s *SeedState) AllowedToHarvest(playerID int32) bool {
	return s.sowerID != 0 && s.sowerID == playerID
}

// HarvestedCrop returns the mature crop id and one crop for now. Manor
// production-rate configuration is outside the current live reward path.
func (s *SeedState) HarvestedCrop() (int32, int) {
	if !s.Seeded() {
		return 0, 0
	}
	return int32(s.seed.MatureID), 1
}
