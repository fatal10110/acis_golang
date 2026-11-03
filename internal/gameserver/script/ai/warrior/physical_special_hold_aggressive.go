package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PhysicalSpecialHoldAggressive is WarriorPhysicalSpecialHoldAggressive: a
// WarriorPhysicalSpecial that stays where it spawned, walking back there
// when created or idle and keeping still once there. It attacks the
// players and summons it sees without moving, and answers a hit, a clan
// call or the kill of a summon with a physical special skill from close
// by, or DD magic from farther, and hate. Hit, it calls for help (event
// 10001) within its HelpCastRange; it ignores every script event.
func PhysicalSpecialHoldAggressive() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22137, 22138, 22194}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if e.Creature.Playable() {
				e.NPC.AddAttackDesireHold(e.Creature, 50)
			}
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			// Always true: no intention is both attack and cast.
			if n.Intention() != script.IntentionAttack || n.Intention() != script.IntentionCast {
				s.BroadcastScriptEvent(n, 10001, attacker.ObjectID(), int(n.IntParamOr("HelpCastRange", 400)))
			}
			if attacker.Playable() {
				holdStrike(s, n, attacker)
				n.AddAttackDesireHold(attacker, damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))*100)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				holdStrike(s, called, attacker)
				called.AddAttackDesireHold(attacker, damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))*100)
			}
		},
		OnScriptEvent: func(*script.Script, script.ScriptEvent) {},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			n := e.NPC
			if at := n.SpawnLocation(); at.X != int(e.X) || at.Y != int(e.Y) || at.Z != int(e.Z) {
				n.AddMoveToDesire(at, 30)
			} else {
				n.AddDoNothingDesire(40, 30)
			}
		},
		OnAttackFinished: func(s *script.Script, e script.AttackFinished) {
			holdAvengeSummon(s, e.NPC, e.Target)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			holdAvengeSummon(s, e.NPC, e.Creature)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if s.Skill(e.Skill).AggroPoints() > 0 {
				n.AddAttackDesireHold(caster, damageHate(n, 1, defaultnpc.HateRatio(n, caster))*150)
			}
		},
	})}
}

// holdStrike makes n, which stays where it is, want to strike target:
// with its physical special skill on a draw under 33 from closer than 80,
// else with its DD magic. A skill its template lists none of is cast by
// no one.
func holdStrike(s *script.Script, n *script.NPC, target script.Creature) {
	if n.Distance2D(target) < 80 {
		if s.Rnd(100) < 33 {
			if ref, ok := n.SkillByType(npc.SkillTypePhysicalSpecial); ok {
				n.AddCastDesireHold(target, ref, 1000000)
			}
		}
	} else if ref, ok := n.SkillByType(npc.SkillTypeDDMagic); ok {
		n.AddCastDesireHold(target, ref, 1000000)
	}
}

// holdAvengeSummon makes n turn on the owner of target, a summon it has
// just killed: holdStrike and hate 50.
func holdAvengeSummon(s *script.Script, n *script.NPC, target script.Creature) {
	if !target.Dead() || !target.Summon() {
		return
	}
	owner := script.ActingPlayer(target)
	if owner == nil {
		return
	}
	holdStrike(s, n, owner)
	n.AddAttackDesireHold(owner, 50)
}
