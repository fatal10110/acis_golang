package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ npc.ScriptHooks = (*Registry)(nil)

// HostileAttacked runs the attacked hook of every script bound to h's
// template, in list order: attacker attacked h for damage, with sk when a
// skill did it.
func (r *Registry) HostileAttacked(h *npc.Hostile, attacker attackable.Combatant, damage int32, sk skill.Ref) {
	list := r.scripts(int32(h.NpcID()), EventAttacked)
	if len(list) == 0 {
		return
	}
	e := Attacked{NPC: NewNPC(h), Attacker: creatureOf(attacker), Damage: damage, Skill: sk}
	for _, s := range list {
		r.run(s, hookAttacked, func() { s.Hooks.Attacked(s, e) })
	}
}

// HostilePartyAttacked runs the party-attacked hook of every script bound
// to called's template, in list order: caller, attacked by target for
// damage, called its party member called.
func (r *Registry) HostilePartyAttacked(caller, called *npc.Hostile, target attackable.Combatant, damage int32) {
	list := r.scripts(int32(called.NpcID()), EventPartyAttacked)
	if len(list) == 0 {
		return
	}
	e := PartyAttacked{Caller: NewNPC(caller), Called: NewNPC(called), Target: creatureOf(target), Damage: damage}
	for _, s := range list {
		r.run(s, hookPartyAttacked, func() { s.Hooks.PartyAttacked(s, e) })
	}
}
