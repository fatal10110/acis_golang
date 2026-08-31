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
	if h.aiInt("MovingAttack", 1) != 1 {
		return
	}
	h.AddDamageHate(target, 0, float64(aggro)*partyAttackedWeight)
	h.AddAttackDesire(target, float64(aggro)*partyAttackedWeight)
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
