package npc

import (
	"sync"
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

// SeedState is one hostile life's manor seed lifecycle. Sowers and
// harvesters act from their own queues while the killer's queue reads it for
// rewards, so every method takes mu.
type SeedState struct {
	mu        sync.Mutex
	sowerID   int32
	seed      manor.Seed
	harvested bool
}

// Seeded reports whether this hostile was sown during its current life.
func (s *SeedState) Seeded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sowerID != 0
}

// Sow records the sower and seed carried by this hostile unless this life
// was already sown. It reports whether sowerID's seed took.
func (s *SeedState) Sow(sowerID int32, seed manor.Seed) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sowerID != 0 {
		return false
	}
	s.sowerID = sowerID
	s.seed = seed
	s.harvested = false
	return true
}

// Harvested reports whether this seeded hostile was already harvested.
func (s *SeedState) Harvested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.harvested
}

// MarkHarvested marks this seeded hostile's crop as consumed.
func (s *SeedState) MarkHarvested() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.harvested = true
}

// ClaimHarvest marks the crop consumed for playerID when this life is sown,
// not yet harvested, and playerID may harvest it. It reports whether
// playerID took the crop; at most one caller per life does.
func (s *SeedState) ClaimHarvest(playerID int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sowerID == 0 || s.harvested || !s.allowedToHarvest(playerID) {
		return false
	}
	s.harvested = true
	return true
}

// AllowedToHarvest currently permits only the original sower. Party sharing
// is deferred until live party membership is available.
func (s *SeedState) AllowedToHarvest(playerID int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowedToHarvest(playerID)
}

func (s *SeedState) allowedToHarvest(playerID int32) bool {
	return s.sowerID != 0 && s.sowerID == playerID
}

// HarvestedCrop returns the mature crop id and one crop for now. Manor
// production-rate configuration is outside the current live reward path.
func (s *SeedState) HarvestedCrop() (int32, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sowerID == 0 {
		return 0, 0
	}
	return int32(s.seed.MatureID), 1
}
