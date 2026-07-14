package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// MaxHP returns this NPC's maximum hit points, from its template.
func (h *Hostile) MaxHP() int {
	return int(h.Instance.Template.HPMax)
}

// CurrentHP returns this NPC's live hit points.
func (h *Hostile) CurrentHP() int {
	return int(h.health.Current())
}

// SetCurrentHP overrides this NPC's live hit points, e.g. to restore a
// persisted value at spawn time instead of starting at MaxHP. It has no
// effect once this NPC has already died.
func (h *Hostile) SetCurrentHP(hp int) {
	h.health.SetCurrent(float64(hp))
}

// TakeDamage applies dmg physical damage from attacker, clamping at zero,
// and — the first time it reaches zero — runs this NPC's death sequence,
// passing the reward hook set via SetRewarder (nil if none was set). It
// reports whether this call newly killed the NPC.
func (h *Hostile) TakeDamage(dmg int, attacker creature.DeathActor) bool {
	if dmg > 0 {
		if combatant, ok := attacker.(attackable.Combatant); ok {
			h.AddDamageHate(combatant, float64(dmg), float64(dmg))
		}
	}
	if !h.health.Damage(dmg) {
		return false
	}
	return h.Die(attacker, h.rewards)
}
