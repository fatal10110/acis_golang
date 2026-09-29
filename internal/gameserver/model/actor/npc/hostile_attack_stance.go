package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// InCombat reports whether this NPC holds an attack stance.
func (h *Hostile) InCombat() bool {
	return h.inCombat.Load()
}

// SetInCombat records whether this NPC holds an attack stance and reports
// whether that changed. The stance tracker clears it when the stance
// expires.
func (h *Hostile) SetInCombat(inCombat bool) bool {
	return h.inCombat.Swap(inCombat) != inCombat
}

// EnterAttackStance reports that this NPC landed a damaging hit or finished
// an offensive cast whose launch resolved a target: it enters its attack
// stance, or refreshes the one it holds.
func (h *Hostile) EnterAttackStance() {
	h.emit(event.AttackStanceRequested{})
}

// NotifyAttacked reports a damaging physical hit, or an offensive skill,
// from attacker reaching this NPC: it enters its attack stance. Recording
// attacker for rewards and aggression is the damage and cast paths' own
// work.
func (h *Hostile) NotifyAttacked(attacker attackable.Combatant) {
	h.emit(event.Attacked{Attacker: attacker})
}

// NotifyEvaded reports a physical hit from attacker that missed this NPC,
// which does not react to it.
func (h *Hostile) NotifyEvaded(attackable.Combatant) {}

// BroadcastAutoAttackStop reports that this NPC's attack stance expired.
func (h *Hostile) BroadcastAutoAttackStop() {
	h.emit(event.AutoAttackStopped{})
}
