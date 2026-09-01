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
