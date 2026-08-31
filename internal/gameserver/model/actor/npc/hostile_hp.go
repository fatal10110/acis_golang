package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// MaxHP returns this NPC's calculated maximum hit points (CreatureStatus.
// getMaxHp: calcStat(MAX_HP, template base) — not the raw template value).
func (h *Hostile) MaxHP() int {
	return int(h.MaxHPValue())
}

// CurrentHP returns this NPC's live hit points.
func (h *Hostile) CurrentHP() int {
	return int(h.health.Current())
}

// SetCurrentHP overrides this NPC's live hit points, clamped to [0,
// calculated MaxHP], e.g. to restore a persisted value at spawn time
// instead of starting at MaxHP. It has no effect once this NPC has already
// died.
func (h *Hostile) SetCurrentHP(hp int) {
	if max := h.MaxHP(); hp > max {
		hp = max
	}
	if hp < 0 {
		hp = 0
	}
	h.health.SetCurrent(float64(hp))
}

// CurrentMP returns this NPC's live mana points, int-truncated to match the
// persisted spawn_data.current_mp contract.
func (h *Hostile) CurrentMP() int {
	return int(h.MPValue())
}

// SetCurrentMP overrides this NPC's live mana points, clamped to [0,
// calculated MaxMP], e.g. to restore a persisted value at spawn time instead
// of starting at MaxMP.
func (h *Hostile) SetCurrentMP(mp int) {
	if max := int(h.MaxMPValue()); mp > max {
		mp = max
	}
	if mp < 0 {
		mp = 0
	}
	h.mpMu.Lock()
	defer h.mpMu.Unlock()
	h.mp = float64(mp)
}

// TakeDamage applies dmg physical damage from attacker, clamping at zero,
// broadcasts the resulting HP to nearby observers, and — the first time it
// reaches zero — runs this NPC's death sequence, passing the reward hook
// set via SetRewarder (nil if none was set). It reports whether this call
// newly killed the NPC. A hit against an already-dead NPC is a no-op: no
// damage is applied and no status is broadcast.
func (h *Hostile) TakeDamage(dmg int, attacker creature.DeathActor) bool {
	if h.AlikeDead() {
		return false
	}
	h.testOverhit(attacker, float64(dmg))
	if dmg > 0 {
		if combatant, ok := attacker.(attackable.Combatant); ok {
			h.AddCombatDamageHate(combatant, float64(dmg))
			h.RollAttackedShotRecharge()
			h.propagatePartyAttacked(h, combatant, dmg, false)
		}
		h.applyNonConsumptionDamageEffects(false)
	}
	newlyDead := h.health.Damage(dmg)
	if err := h.BroadcastStatus(); err != nil {
		h.log.Warn().Err(err).Msg("npc: status broadcast")
	}
	if !newlyDead {
		return false
	}
	return h.Die(attacker, h.rewards)
}
