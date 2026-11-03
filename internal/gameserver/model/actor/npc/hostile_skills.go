package npc

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SkillDisabled reports whether h may not use the skill of reuse key key.
// With no skill waiting for its reuse, it may. Otherwise a stun,
// meditation, sleep, paralysis or fear locks every skill, and failing that
// key's own reuse wait decides; an expired wait is forgotten as it is read.
func (h *Hostile) SkillDisabled(key int32) bool {
	h.skillMu.Lock()
	tracked := len(h.disabledSkills) > 0
	h.skillMu.Unlock()
	if !tracked {
		return false
	}
	// The lock is read before key's own wait, whose check forgets an
	// expired wait: an expired wait still tracked leaves the lock in force.
	// The effect list's lock is taken with skillMu released.
	if h.skillsLocked() {
		return true
	}
	h.skillMu.Lock()
	defer h.skillMu.Unlock()
	expiresAt, ok := h.disabledSkills[key]
	if !ok {
		return false
	}
	if h.Now().Before(expiresAt) {
		return true
	}
	delete(h.disabledSkills, key)
	return false
}

// skillsLocked reports whether crowd control keeps h from every skill: a
// stun, meditation, sleep, paralysis or fear.
func (h *Hostile) skillsLocked() bool {
	return h.Stunned() || h.ImmobileUntilAttacked() || h.Sleeping() || h.Paralyzed() || h.Afraid()
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
	h.disabledSkills[key] = h.Now().Add(delay)
}

// AddSkillReuse installs an NPC-local skill reuse delay. Hostiles do not
// persist skill reuse timers, so ref is intentionally not stored.
func (h *Hostile) AddSkillReuse(_ modelskill.Ref, key int32, delay time.Duration) {
	h.DisableSkill(key, delay)
}
