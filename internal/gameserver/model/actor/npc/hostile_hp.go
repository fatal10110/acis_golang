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

// PublishHP hands send this NPC's current HP when the players targeting it
// must be sent it, under the health-bar lock (creature.HPBar.Publish).
// Callers invoke it only when at least one player is targeting this NPC: an
// unwatched NPC's bar state stays where it was last reported.
func (h *Hostile) PublishHP(send func(hp int)) {
	h.hpBar.Publish(h.health.Current, float64(h.MaxHP()), send)
}

// CurrentMP returns this NPC's live mana points, int-truncated to match the
// persisted spawn_data.current_mp contract.
func (h *Hostile) CurrentMP() int {
	return int(h.MPValue())
}

// SetCurrentMP overrides this NPC's live mana points, clamped to [0,
// calculated MaxMP], e.g. to restore a persisted value at spawn time instead
// of starting at MaxMP. It has no effect once this NPC has died.
func (h *Hostile) SetCurrentMP(mp int) {
	if max := int(h.MaxMPValue()); mp > max {
		mp = max
	}
	if mp < 0 {
		mp = 0
	}
	h.whileAliveMP(func() { h.mp = float64(mp) })
}

// TakeDamage applies dmg physical damage from attacker, clamping at zero,
// broadcasts the resulting HP to nearby observers, and — the first time it
// reaches zero — runs this NPC's death sequence, passing the reward hook
// installed by Attach (nil if none was). It reports whether this call
// newly killed the NPC. A hit against an already-dead NPC is a no-op: no
// damage is applied and no status is broadcast. Hate, the shot-recharge
// roll, and the party/minion attacked call always run for a live hit,
// unconditionally, one layer above the invul/damage-permission guard. The
// overhit test runs first, then that guard drops only the HP change
// itself: an invulnerable NPC, or
// one hit by an attacker without damage permission, still aggroes and calls
// its party, but takes no damage. The hit's cast-break roll is the
// attacker's to run, through BreakCastOnDamage, once the hit's reflected
// and absorbed damage have applied.
func (h *Hostile) TakeDamage(dmg int, attacker attackable.Combatant) bool {
	if h.AlikeDead() {
		return false
	}
	creature.InterruptDuelOnNPCHit(attacker)
	h.testOverhit(attacker, float64(dmg))
	if dmg > 0 {
		h.RecordAttacker(attacker)
		h.registerHit(attacker, float64(dmg), false)
	}
	if h.Invul() || !creature.CanDealDamage(attacker) {
		return false
	}
	if dmg > 0 {
		h.applyNonConsumptionDamageEffects(false)
	}
	newlyDead := h.health.Damage(dmg)
	h.BroadcastStatus()
	if !newlyDead {
		return false
	}
	return h.Die(attacker, h.rewards)
}
