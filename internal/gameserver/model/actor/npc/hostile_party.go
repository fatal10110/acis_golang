package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

const partyAttackedWeight = 1.0

// NotifyAggression records the incoming aggression on this NPC and fans it
// out to the master/minion party so escorts assist the same target.
func (h *Hostile) NotifyAggression(source creature.DeathActor, power int) {
	combatant, ok := source.(attackable.Combatant)
	if !ok {
		return
	}
	h.AddAttackDesire(combatant, float64(power))
	h.propagatePartyAttacked(h, combatant, power, true)
}

// registerHit records hate, the shot-recharge roll, and the party/minion
// attacked call for a live hit with positive amount — the block
// TakeDamage, ReduceHP, and ReduceHPByDOT all run unconditionally one layer
// above the invul/damage-permission guard, matching Npc.reduceCurrentHp
// (Npc.java:390-464). isDOT selects ReduceHPByDOT's zero-weight hate call
// plus its flat attack-desire (Npc.java:395's unconditional
// addDamageHate(attacker, damage, 0)) instead of TakeDamage/ReduceHP's
// damage-weighted combat hate. A no-op when attacker isn't a Combatant
// (e.g. an environmental DOT source). Pulled out after this exact block
// drifted out of order between copies twice (#2326, #2328) — one place to
// keep the ordering right.
func (h *Hostile) registerHit(attacker any, amount float64, isDOT bool) {
	combatant, ok := attacker.(attackable.Combatant)
	if !ok {
		return
	}
	if isDOT {
		h.AddDamageHate(combatant, amount, 0)
		h.AddAttackDesire(combatant, 200)
	} else {
		h.AddCombatDamageHate(combatant, amount)
	}
	h.RollAttackedShotRecharge()
	h.propagatePartyAttacked(h, combatant, int(amount), false)
}

func (h *Hostile) inParty() bool {
	return h.IsMaster() || h.Master() != nil
}

func (h *Hostile) partyMinions() []*Hostile {
	if master := h.Master(); master != nil {
		return master.Minions()
	}
	return h.Minions()
}

func (h *Hostile) propagatePartyAttacked(caller *Hostile, target attackable.Combatant, aggro int, includeSelfInMinionLoop bool) {
	if !h.inParty() {
		return
	}
	h.reactPartyAttacked(caller, target, aggro)
	if master := h.Master(); master != nil && !master.AlikeDead() {
		master.reactPartyAttacked(caller, target, aggro)
	}
	for _, minion := range h.partyMinions() {
		if minion.AlikeDead() {
			continue
		}
		if !includeSelfInMinionLoop && minion == caller {
			continue
		}
		minion.reactPartyAttacked(caller, target, aggro)
	}
}

func (h *Hostile) reactPartyAttacked(caller *Hostile, target attackable.Combatant, aggro int) {
	if h.aiInt("IsHealer", 0) == 1 {
		return
	}
	partyType := h.aiInt("Party_Type", 0)
	if partyType == 0 || h.Master() == nil {
		return
	}
	loyalty := h.aiInt("Party_Loyalty", 0)
	assist := (partyType == 1 && (loyalty == 0 || loyalty == 1)) ||
		(partyType == 1 && loyalty == 2 && caller == h.Master()) ||
		(partyType == 2 && caller != h.Master())
	if !assist {
		return
	}
	weight := float64(aggro) * partyAttackedWeight
	top := h.brain.TopDesireTarget()
	if h.aiInt("MovingAttack", 1) == 1 {
		h.queueMovingPartyAttack(target, top, weight)
		return
	}
	if h.canAutoAttack(target) {
		h.queuePartyAttack(target, weight, true)
		return
	}
	if samePartyTarget(top, target) {
		h.RemoveAttackDesire(top)
	}
}

func (h *Hostile) queueMovingPartyAttack(target, top attackable.Combatant, weight float64) {
	h.queuePartyAttack(target, weight, false)
	if top == nil {
		return
	}
	if h.GeoPathFailCount() > 10 && samePartyTarget(target, top) && h.hpRatio() < 1 {
		if pos, ok := combatantLocation(target); ok {
			h.TeleportTo(pos)
			h.ResetGeoPathFailCount()
		}
	}
	if h.Rooted() && partyDistance2D(h, top) > 40 {
		if !h.canAutoAttack(top) {
			h.RemoveAttackDesire(top)
		}
		h.queuePartyAttack(target, weight, false)
	}
}

func (h *Hostile) queuePartyAttack(target attackable.Combatant, weight float64, hold bool) {
	h.AddDamageHate(target, 0, weight)
	if hold {
		h.AddAttackDesireHold(target, weight)
		return
	}
	h.AddAttackDesire(target, weight)
}

func (h *Hostile) canAutoAttack(target attackable.Combatant) bool {
	if h.Instance == nil || h.Instance.Template == nil {
		return false
	}
	return h.AutoAttackTargetValid(target, h.Instance.Template.AggroRange, false)
}

func (h *Hostile) hpRatio() float64 {
	maxHP := h.MaxHPValue()
	if maxHP <= 0 {
		return 0
	}
	return h.HP() / maxHP
}

func samePartyTarget(a, b attackable.Combatant) bool {
	return a != nil && b != nil && a.ObjectID() == b.ObjectID()
}

func partyDistance2D(h *Hostile, other attackable.Combatant) float64 {
	pos, ok := combatantLocation(other)
	if !ok {
		return 0
	}
	return h.location().Distance2D(pos)
}

func (h *Hostile) aiInt(key string, def int) int {
	if h.Instance == nil || h.Instance.Template == nil || h.Instance.Template.AIParams == nil {
		return def
	}
	v, err := h.Instance.Template.AIParams.GetIntDefault(key, def)
	if err != nil {
		return def
	}
	return v
}
