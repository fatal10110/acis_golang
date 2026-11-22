package npc

import (
	"math"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// overhitState records one lethal overhit strike against a hostile NPC.
// A playable caster's overhit skill arms it; the next HP reduction then
// either stores the excess damage or clears the flag. mu is taken from
// other actors' queues: the caster arms it (EnableOverhit), the attacker's
// HP reduction tests it, and the killer's reward reads it.
type overhitState struct {
	mu       sync.Mutex
	enabled  bool
	damage   float64
	attacker attackable.Combatant
}

func (o *overhitState) set(enabled bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled = enabled
}

func (o *overhitState) test(attacker attackable.Combatant, currentHP, damage float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.enabled {
		return
	}
	if damage <= 0 {
		o.enabled = false
		return
	}
	overhitDamage := (currentHP - damage) * -1
	if overhitDamage < 0 {
		o.enabled = false
		return
	}
	o.damage = overhitDamage
	o.attacker = attacker
}

// overhitBonusCapPercent caps the overhit bonus, as a percentage of the
// kill's normal experience.
const overhitBonusCapPercent = 25

func (o *overhitState) bonusExp(normalExp int64, maxHP float64) int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	percentage := (o.damage * 100) / maxHP
	if percentage > overhitBonusCapPercent {
		percentage = overhitBonusCapPercent
	}
	return int64(math.Round((percentage / 100) * float64(normalExp)))
}

func (o *overhitState) valid(player attackable.Combatant) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.enabled || o.attacker == nil || player == nil {
		return false
	}
	// By object id: a summon's owner and the rewarded player can be
	// different wrappers around the same character.
	acting := overhitActingPlayer(o.attacker)
	return acting != nil && acting.ObjectID() == player.ObjectID()
}

func overhitActingPlayer(a attackable.Combatant) attackable.Combatant {
	if a.Kind() == actor.KindSummon {
		owner, _ := a.Owner()
		return owner
	}
	return a
}

// EnableOverhit arms this NPC for an overhit check on the next HP reduction.
func (h *Hostile) EnableOverhit() {
	h.ensureLoot().overhit.set(true)
}

// testOverhit settles an armed over-hit against an HP reduction; a life
// no over-hit skill armed has nothing to settle.
func (h *Hostile) testOverhit(attacker attackable.Combatant, damage float64) {
	if l := h.loot.Load(); l != nil {
		l.overhit.test(attacker, h.HP(), damage)
	}
}

// OverhitValid reports whether player is the acting player of a successful
// overhit strike stored on this NPC.
func (h *Hostile) OverhitValid(player attackable.Combatant) bool {
	l := h.loot.Load()
	return l != nil && l.overhit.valid(player)
}

// OverhitBonus returns the rounded XP bonus for a successful overhit against
// normalExp, capped at 25% of that amount from excess damage vs max HP.
func (h *Hostile) OverhitBonus(normalExp int64) int64 {
	l := h.loot.Load()
	if l == nil {
		return 0
	}
	return l.overhit.bonusExp(normalExp, h.MaxHPValue())
}
