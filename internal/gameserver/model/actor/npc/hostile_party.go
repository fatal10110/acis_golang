package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const partyAttackedWeight = 1.0

// flatAttackedHateWeight is the fallback ATTACKED-event attack Desire
// weight for a non-Playable attacker, or while the individual AI script
// that would derive it hasn't landed yet (#185, M10 - Content port at
// scale). This is a Go-only divergence, not an approximation of the
// specified behavior: both of the Warrior script's attacked-hook
// attack-desire adds, and the WarriorBase script's, apply only to a
// Playable attacker — no ATTACKED hate at all is specified for a
// non-Playable attacker. Pre-existing (unchanged by #2346): before
// that PR the threat table used hate=damage here, not this flat weight.
const flatAttackedHateWeight = 200

// attackedHateWeight approximates the specified per-hit ATTACKED-event
// hate weight, which comes from the NPC's assigned individual AI script
// (#185, M10): damage is truncated to an int before the script's attacked
// hook sees it, and the Warrior script computes that truncated
// damage/(level+7)*100 for a Playable attacker, further scaled by the
// script's hate ratio — the
// SetHateGroup/SetHateOccupation/SetHateRace AI-param ratios, which apply
// only to a Player attacker (not every Playable) — that neither Go's
// Template nor the script engine support yet, so dropped here. Other script
// families (WizardBase, MonsterBehavior, LV3Monster, …) use different
// formulas Go hasn't ported. Until #185 lands that per-script dispatch,
// every Hostile uses this one damage-proportional formula for Playable
// attackers instead (narrowing damage to an int the same way, so a sub-1
// DOT tick contributes zero hate and an unbounded
// zero-defence hit a finite MaxInt32-sized one), and falls back to the
// flat pre-existing weight for everything else — an approximation, not the
// full per-script behavior. It runs only for an NPC with no bound behavior:
// a bound behavior's attacked hook decides the hate instead.
func (h *Hostile) attackedHateWeight(attacker attackable.Combatant, damage float64) float64 {
	if !creature.Playable(attacker) {
		return flatAttackedHateWeight
	}
	return float64(commons.JavaInt(damage)) / (float64(h.Level()) + 7) * 100
}

// NotifyAggression is an aggression effect source landed on this NPC with
// power: no HP change and no skill. The attacked hooks run with power as
// the damage, then the party is called: the NPC itself, its master (alive),
// and every live minion of the party, the NPC itself again when it is one.
// Without a bound behavior, power first goes through attackedHateWeight
// into an attack desire.
func (h *Hostile) NotifyAggression(source attackable.Combatant, power int) {
	if source == nil {
		return
	}
	if !h.behaves() {
		h.AddAttackDesire(source, h.attackedHateWeight(source, float64(power)))
	}
	h.raiseAttacked(source, int32(power), skill.Ref{})
	h.propagatePartyAttacked(h, source, power, true, true)
}

// SkillAttacked is caster's offensive skill sk landing on this NPC, once
// its effects applied, for a skill that is a debuff or carries aggro
// points. The attacked hooks run with max(120, aggro points) as the damage,
// then the party is called as for a hit. A dead NPC is called too, and
// nothing else reacts: a skill's call has no built-in reaction.
func (h *Hostile) SkillAttacked(caster attackable.Combatant, sk skill.Definition) {
	if caster == nil {
		return
	}
	value := max(120, sk.AggroPoints)
	h.raiseAttacked(caster, int32(value), skill.Ref{ID: sk.ID, Level: sk.Level})
	h.propagatePartyAttacked(h, caster, value, false, false)
}

// registerHit records hate, the shot-recharge roll, and the attacked hooks
// and party call for a live hit, one layer above the invul/damage-permission
// guard. ReduceHP and ReduceHPByDOT run it whatever the amount, a
// zero-damage hit included (there is no damage check at that layer).
// TakeDamage only ever sees positive damage, since the auto-attack path
// drops zero hits before reaching it, so its dmg > 0 gate never skips a
// real hit. sk is the skill that dealt the hit, the zero Ref for none. A
// no-op when attacker isn't a Combatant (e.g. an environmental DOT source).
// Pulled out after this exact block drifted out of order between copies
// twice (#2326, #2328) — one place to keep the ordering right. The hit
// observer sees the hit first, ahead of everything it causes.
//
// The damage enters the threat table at zero hate. Without a bound
// behavior the built-in reactions follow: an attack desire weighted by
// attackedHateWeight (isDOT adds it apart from the damage, as
// ReduceHPByDOT does; otherwise AddCombatDamageHate writes both), and the
// shot-recharge roll. The attacked hooks then run with the damage truncated
// to an int, and the party is called.
func (h *Hostile) registerHit(combatant attackable.Combatant, amount float64, isDOT bool, sk skill.Ref) {
	if combatant == nil {
		return
	}
	if h.hits != nil {
		h.hits.Hit(combatant)
	}
	switch {
	case h.behaves():
		h.AddDamageHate(combatant, amount, 0)
	case isDOT:
		h.AddDamageHate(combatant, amount, 0)
		h.AddAttackDesire(combatant, h.attackedHateWeight(combatant, amount))
		h.RollAttackedShotRecharge()
	default:
		h.AddCombatDamageHate(combatant, amount)
		h.RollAttackedShotRecharge()
	}
	damage := int(commons.JavaInt(amount))
	h.raiseAttacked(combatant, int32(damage), sk)
	h.propagatePartyAttacked(h, combatant, damage, false, true)
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

// propagatePartyAttacked calls the party of caller, an NPC in a party
// attacked by target for damage: caller itself, then its master when
// alive, then every live minion of the party. The minion loop skips caller
// unless includeSelfInMinionLoop, as an aggression effect's does not.
// assist turns on the built-in party assist of the called NPCs that have
// no bound behavior.
func (h *Hostile) propagatePartyAttacked(caller *Hostile, target attackable.Combatant, damage int, includeSelfInMinionLoop, assist bool) {
	if !h.inParty() {
		return
	}
	h.partyAttacked(caller, target, damage, assist)
	if master := h.Master(); master != nil && !master.AlikeDead() {
		master.partyAttacked(caller, target, damage, assist)
	}
	for _, minion := range h.partyMinions() {
		if minion.AlikeDead() {
			continue
		}
		if !includeSelfInMinionLoop && minion == caller {
			continue
		}
		minion.partyAttacked(caller, target, damage, assist)
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
		h.TeleportTo(combatantLocation(target))
		h.ResetGeoPathFailCount()
	}
	if h.Rooted() && partyDistance2D(h, top) > 40 {
		if !h.canAutoAttack(top) {
			h.RemoveAttackDesire(top)
		}
		h.queuePartyAttack(target, weight, false)
	}
}

// queuePartyAttack is a forced attack: a single attack-desire add
// (moveToTarget or hold) that feeds the threat table itself — the
// party/minion assist path has no separate damage-hate add.
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
	return h.location().Distance2D(combatantLocation(other))
}

func (h *Hostile) aiInt(key string, def int) int {
	return int(h.AIInt(key, int32(def)))
}
