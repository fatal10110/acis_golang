package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

const partyAttackedWeight = 1.0

// flatAttackedHateWeight is the fallback ATTACKED-event attack Desire
// weight for a non-Playable attacker, or while the individual AI script
// that would derive it hasn't landed yet (#185, M10 - Content port at
// scale). This is a Go-only divergence, not an approximation of Java: both
// of Warrior.onAttacked's addAttackDesire calls, and WarriorBase.onAttacked's,
// sit inside "if (attacker instanceof Playable)" (Warrior.java:392-397,
// WarriorBase.java:100-102) — the reference adds zero ATTACKED hate at all
// for a non-Playable attacker. Pre-existing (unchanged by #2346): before
// that PR the threat table used hate=damage here, not this flat weight.
const flatAttackedHateWeight = 200

// attackedHateWeight approximates the reference's per-hit ATTACKED-event
// hate weight. The reference derives this via the NPC's assigned
// individual AI script (#185, M10): Warrior.onAttacked (Warrior.java:387-397)
// computes damage/(level+7)*100 for a Playable attacker, further scaled by
// getHateRatio (DefaultNpc.java:111-131) — the SetHateGroup/SetHateOccupation/
// SetHateRace AI-param ratios, which apply only to a Player attacker (not
// every Playable) — that neither Go's Template nor the script engine
// support yet, so dropped here. Other script families (WizardBase,
// MonsterBehavior, LV3Monster, …) use different formulas Go hasn't ported.
// Until #185 lands that per-script dispatch, every Hostile uses this one
// damage-proportional formula for Playable attackers instead, and falls
// back to the flat pre-existing weight for everything else — an
// approximation, not the full per-script behavior.
func (h *Hostile) attackedHateWeight(attacker attackable.Combatant, damage float64) float64 {
	if !creature.Playable(attacker) {
		return flatAttackedHateWeight
	}
	return damage / (float64(h.Level()) + 7) * 100
}

// NotifyAggression records the incoming aggression on this NPC and fans it
// out to the master/minion party so escorts assist the same target. power
// goes through attackedHateWeight before reaching the threat table, matching
// AttackableAI.onEvtAggression (AttackableAI.java:119-123), which routes the
// AGGRESSION event into the same per-script onAttacked(actor, target, aggro,
// null) chain a real hit uses, with aggro standing in for damage.
func (h *Hostile) NotifyAggression(source creature.DeathActor, power int) {
	combatant, ok := source.(attackable.Combatant)
	if !ok {
		return
	}
	h.AddAttackDesire(combatant, h.attackedHateWeight(combatant, float64(power)))
	h.propagatePartyAttacked(h, combatant, power, true)
}

// registerHit records hate, the shot-recharge roll, and the party/minion
// attacked call for a live hit with positive amount — the block
// TakeDamage, ReduceHP, and ReduceHPByDOT all run unconditionally one layer
// above the invul/damage-permission guard, matching Npc.reduceCurrentHp
// (Npc.java:390-464). isDOT selects ReduceHPByDOT's zero-weight hate call
// (Npc.java:395's unconditional addDamageHate(attacker, damage, 0)) plus its
// own attackedHateWeight-derived attack Desire, instead of
// AddCombatDamageHate's combined write for TakeDamage/ReduceHP. A no-op when
// attacker isn't a Combatant (e.g. an environmental DOT source). Pulled out
// after this exact block drifted out of order between copies twice (#2326,
// #2328) — one place to keep the ordering right.
func (h *Hostile) registerHit(attacker any, amount float64, isDOT bool) {
	combatant, ok := attacker.(attackable.Combatant)
	if !ok {
		return
	}
	if isDOT {
		h.AddDamageHate(combatant, amount, 0)
		h.AddAttackDesire(combatant, h.attackedHateWeight(combatant, amount))
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

// queuePartyAttack mirrors Npc.forceAttack: a single addAttackDesire call
// (moveToTarget or hold) that feeds the threat table itself — matching the
// reference's absence of any separate addDamageHate call in the party/minion
// assist path.
func (h *Hostile) queuePartyAttack(target attackable.Combatant, weight float64, hold bool) {
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
