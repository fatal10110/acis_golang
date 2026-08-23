package npc

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SkillDisabled reports whether key is still waiting for its reuse delay.
func (h *Hostile) SkillDisabled(key int32) bool {
	h.skillMu.Lock()
	defer h.skillMu.Unlock()
	expiresAt, ok := h.disabledSkills[key]
	if !ok {
		return false
	}
	if time.Now().Before(expiresAt) {
		return true
	}
	delete(h.disabledSkills, key)
	return false
}

// DisableSkill marks key unusable until delay elapses.
func (h *Hostile) DisableSkill(key int32, delay time.Duration) {
	if delay <= 0 {
		return
	}
	h.skillMu.Lock()
	defer h.skillMu.Unlock()
	if h.disabledSkills == nil {
		h.disabledSkills = make(map[int32]time.Time)
	}
	h.disabledSkills[key] = time.Now().Add(delay)
}

// AddSkillReuse installs an NPC-local skill reuse delay. Hostiles do not
// persist skill reuse timers, so ref is intentionally not stored.
func (h *Hostile) AddSkillReuse(_ modelskill.Ref, key int32, delay time.Duration) {
	h.DisableSkill(key, delay)
}
