package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// MaxDriftRange is the drift range of an NPC spawned at a point: how far it
// may stray from that point before it is out of its territory.
const MaxDriftRange = defaultDriftRange

// CanAutoAttack reports whether target is a legal automatic-combat target
// for h at its template's aggro range: the siege guard's own rule for a
// siege guard, else AutoAttackTargetValid with no peaceful allowance.
func (h *Hostile) CanAutoAttack(target attackable.Combatant) bool {
	if h.SiegeGuard() {
		return h.siegeGuardAutoAttackTargetValid(target)
	}
	return h.AutoAttackTargetValid(target, h.Instance.Template.AggroRange, false)
}
