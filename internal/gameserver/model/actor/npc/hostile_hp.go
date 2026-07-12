package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"

// MaxHP returns this NPC's maximum hit points, from its template.
func (h *Hostile) MaxHP() int {
	return int(h.Instance.Template.HPMax)
}

// CurrentHP returns this NPC's live hit points.
func (h *Hostile) CurrentHP() int {
	h.hpMu.Lock()
	defer h.hpMu.Unlock()
	return h.hp
}

// TakeDamage applies dmg physical damage from attacker, clamping at zero,
// and — the first time it reaches zero — runs this NPC's death sequence
// with a nil reward hook (see Die's own doc comment: reward calculation is
// the kill-reward system's job, wired in by whatever calls Die with a real
// Rewarder once this NPC is spawned live). It reports whether this call
// newly killed the NPC.
func (h *Hostile) TakeDamage(dmg int, attacker creature.DeathActor) bool {
	if dmg < 0 {
		dmg = 0
	}

	h.hpMu.Lock()
	if h.hp <= 0 {
		h.hpMu.Unlock()
		return false
	}
	h.hp -= dmg
	dead := h.hp <= 0
	h.hpMu.Unlock()

	if !dead {
		return false
	}
	return h.Die(attacker, nil)
}
