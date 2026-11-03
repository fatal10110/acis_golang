package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// NotifyAggression is an aggression effect source landed on this NPC with
// power: no HP change and no skill. The attacked hooks run with power as
// the damage, then the party is called: the NPC itself, its master (alive),
// and every live minion of the party, the NPC itself again when it is one.
// Then the NPC makes its clan call to itself only.
func (h *Hostile) NotifyAggression(source attackable.Combatant, power int) {
	if source == nil {
		return
	}
	h.raiseAttacked(source, int32(power), skill.Ref{})
	propagatePartyAttacked(h, source, power, true)
	raiseClanAttacked(h.scripts, h.world, h, source, int32(power), skill.Ref{}, clanAttackedByAggression)
}

// SkillAttacked is caster's offensive skill sk landing on this NPC, once
// its effects applied, for a skill that is a debuff or carries aggro
// points. The attacked hooks run with max(120, aggro points) as the damage,
// then the party is called as for a hit, then the hostile NPCs of its clan
// in range, not the NPC itself. A dead NPC is called too, and nothing else
// reacts: a skill's call has no built-in reaction.
func (h *Hostile) SkillAttacked(caster attackable.Combatant, sk skill.Definition) {
	if caster == nil {
		return
	}
	value := max(120, sk.AggroPoints)
	ref := skill.Ref{ID: sk.ID, Level: sk.Level}
	h.raiseAttacked(caster, int32(value), ref)
	propagatePartyAttacked(h, caster, value, false)
	raiseClanAttacked(h.scripts, h.world, h, caster, int32(value), ref, clanAttackedBySkill)
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
// The damage enters the threat table at zero hate. The attacked hooks then
// run with the damage truncated to an int, the party is called, and then
// the clan: the NPC itself, then every NPC of its clan in range.
func (h *Hostile) registerHit(combatant attackable.Combatant, amount float64, sk skill.Ref) {
	if combatant == nil {
		return
	}
	if h.hits != nil {
		h.hits.Hit(combatant)
	}
	h.AddDamageHate(combatant, amount, 0)
	h.lastAttacker.Store(&attackerRef{c: combatant})
	damage := int(commons.TruncInt32Sat(amount))
	h.raiseAttacked(combatant, int32(damage), sk)
	propagatePartyAttacked(h, combatant, damage, false)
	raiseClanAttacked(h.scripts, h.world, h, combatant, int32(damage), sk, clanAttackedByHit)
}
