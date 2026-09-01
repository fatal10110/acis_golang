package npc

import (
	"math"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// overhitState records one lethal overhit strike against a hostile NPC.
// A playable caster's overhit skill arms it; the next HP reduction then
// either stores the excess damage or clears the flag.
type overhitState struct {
	mu       sync.Mutex
	enabled  bool
	damage   float64
	attacker creature.DeathActor
}

func (o *overhitState) set(enabled bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled = enabled
}

func (o *overhitState) test(attacker creature.DeathActor, currentHP, damage float64) {
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

func (o *overhitState) bonusExp(normalExp int64, maxHP float64) int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	percentage := (o.damage * 100) / maxHP
	if percentage > 25 {
		percentage = 25
	}
	return int64(math.Round((percentage / 100) * float64(normalExp)))
}

func (o *overhitState) valid(player creature.DeathActor) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.enabled || o.attacker == nil || player == nil {
		return false
	}
	acting := overhitActingPlayer(o.attacker)
	return acting != nil && acting == player
}

type overhitOwner interface {
	ActingPlayer() creature.DeathActor
}

func overhitActingPlayer(a creature.DeathActor) creature.DeathActor {
	if owner, ok := a.(overhitOwner); ok {
		return owner.ActingPlayer()
	}
	return a
}

// EnableOverhit arms this NPC for an overhit check on the next HP reduction.
func (h *Hostile) EnableOverhit() {
	h.overhit.set(true)
}

func (h *Hostile) testOverhit(attacker creature.DeathActor, damage float64) {
	h.overhit.test(attacker, h.HP(), damage)
}

// OverhitValid reports whether player is the acting player of a successful
// overhit strike stored on this NPC.
func (h *Hostile) OverhitValid(player creature.DeathActor) bool {
	return h.overhit.valid(player)
}

// OverhitBonus returns the rounded XP bonus for a successful overhit against
// normalExp, capped at 25% of that amount from excess damage vs max HP.
func (h *Hostile) OverhitBonus(normalExp int64) int64 {
	return h.overhit.bonusExp(normalExp, h.MaxHPValue())
}
