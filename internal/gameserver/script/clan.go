package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// FolkAttacked runs the attacked hook of every script bound to f's
// template, in list order: attacker attacked f for damage, with sk when a
// skill did it.
func (r *Registry) FolkAttacked(f *npc.Folk, attacker attackable.Combatant, damage int32, sk skill.Ref) {
	list := r.scripts(int32(f.NpcID()), EventAttacked)
	if len(list) == 0 {
		return
	}
	e := Attacked{NPC: NPCOf(f), Attacker: creatureOf(attacker), Damage: damage, Skill: sk}
	for _, s := range list {
		r.run(s, hookAttacked, func() { s.Hooks.Attacked(s, e) })
	}
}

// ClanAttacked runs the clan-attacked hook of every script bound to
// called's template, in list order: caller, attacked by attacker for
// damage, with sk when a skill did it, called its clan member called.
func (r *Registry) ClanAttacked(caller, called, attacker attackable.Combatant, damage int32, sk skill.Ref) {
	list := r.scripts(npcIDOf(called), EventClanAttacked)
	if len(list) == 0 {
		return
	}
	e := ClanAttacked{Caller: NPCOf(caller), Called: NPCOf(called), Attacker: creatureOf(attacker), Damage: damage, Skill: sk}
	for _, s := range list {
		r.run(s, hookClanAttacked, func() { s.Hooks.ClanAttacked(s, e) })
	}
}

// HostilePartyDied runs the party-died hook of every script bound to
// called's template, in list order: caller, of called's party, has just
// died.
func (r *Registry) HostilePartyDied(caller, called *npc.Hostile) {
	list := r.scripts(int32(called.NpcID()), EventPartyDied)
	if len(list) == 0 {
		return
	}
	e := PartyDied{Caller: NewNPC(caller), Called: NewNPC(called)}
	for _, s := range list {
		r.run(s, hookPartyDied, func() { s.Hooks.PartyDied(s, e) })
	}
}

// ClanDied runs the clan-died hook of every script bound to called's
// template, in list order: caller, killed by killer, told its clan member
// called.
func (r *Registry) ClanDied(caller, called, killer attackable.Combatant) {
	list := r.scripts(npcIDOf(called), EventClanDied)
	if len(list) == 0 {
		return
	}
	e := ClanDied{Caller: NPCOf(caller), Called: NPCOf(called), Killer: creatureOf(killer)}
	for _, s := range list {
		r.run(s, hookClanDied, func() { s.Hooks.ClanDied(s, e) })
	}
}

// npcIDOf returns the template id of the NPC n, 0 for anything else: no
// script is bound to id 0.
func npcIDOf(n attackable.Combatant) int32 {
	switch o := n.(type) {
	case *npc.Hostile:
		return int32(o.NpcID())
	case *npc.Folk:
		return int32(o.NpcID())
	}
	return 0
}
