package script

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// UseSkillFinished runs the skill-finished hook of every script bound to
// n's template, in list order: n's cast of sk on target ended, completed
// when success. n is a hostile or a civilian NPC.
func (r *Registry) UseSkillFinished(n, target attackable.Combatant, sk skill.Ref, success bool) {
	list := r.scripts(npcIDOf(n), EventUseSkillFinished)
	if len(list) == 0 {
		return
	}
	e := UseSkillFinished{NPC: NPCOf(n), Creature: creatureOf(target), Skill: sk, Success: success}
	for _, s := range list {
		r.run(s, hookUseSkillFinished, func() { s.Hooks.UseSkillFinished(s, e) })
	}
}

// HostileAttackFinished runs the attack-finished hook of every script
// bound to h's template, in list order: h's attack on target ended.
func (r *Registry) HostileAttackFinished(h *npc.Hostile, target attackable.Combatant) {
	list := r.scripts(int32(h.NpcID()), EventAttackFinished)
	if len(list) == 0 {
		return
	}
	e := AttackFinished{NPC: NewNPC(h), Target: creatureOf(target)}
	for _, s := range list {
		r.run(s, hookAttackFinished, func() { s.Hooks.AttackFinished(s, e) })
	}
}

// SeeSpell runs the see-spell hook of every script bound to n's template,
// in list order: caster, a player, cast sk on targets near n, through its
// summon when isPet. A door among targets is a Door handle, in its place.
func (r *Registry) SeeSpell(n, caster attackable.Combatant, sk skill.Ref, targets []skilltarget.Actor, isPet bool) {
	list := r.scripts(npcIDOf(n), EventSeeSpell)
	if len(list) == 0 {
		return
	}
	seen := make([]Creature, 0, len(targets))
	for _, t := range targets {
		switch o := t.(type) {
		case attackable.Combatant:
			seen = append(seen, creatureOf(o))
		case *door.Object:
			seen = append(seen, &Door{obj: o, doors: r.doors})
		}
	}
	e := SeeSpell{NPC: NPCOf(n), Caster: PlayerOf(caster), Skill: sk, Targets: seen, IsPet: isPet}
	for _, s := range list {
		r.run(s, hookSeeSpell, func() { s.Hooks.SeeSpell(s, e) })
	}
}

// Spelled runs the spelled hook of every script bound to n's template, in
// list order: sk, not offensive, landed on n from caster, a player.
func (r *Registry) Spelled(n, caster attackable.Combatant, sk skill.Ref) {
	list := r.scripts(npcIDOf(n), EventSpelled)
	if len(list) == 0 {
		return
	}
	e := Spelled{NPC: NPCOf(n), Caster: PlayerOf(caster), Skill: sk}
	for _, s := range list {
		r.run(s, hookSpelled, func() { s.Hooks.Spelled(s, e) })
	}
}

// AbnormalStatusChanged runs the abnormal-status hook of every script that
// sees spells on n, in list order: an effect of caster's sk is about to
// land on n. caster is nil for a sourceless effect.
func (r *Registry) AbnormalStatusChanged(n, caster attackable.Combatant, sk skill.Ref) {
	id := npcIDOf(n)
	if len(r.scripts(id, EventSeeSpell)) == 0 {
		return
	}
	r.abnormalStatusChanged(id, AbnormalStatusChanged{NPC: NPCOf(n), Caster: creatureOf(caster), Skill: sk})
}
