package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PatrolWeaponEquipped is WarriorPatrolWeaponEquipped: a wandering
// WarriorPhysicalSpecial that debuffs the armed players it sees and, the
// debuff cast, attacks the last of them, the first time since it last had
// nothing to do shouting as it does. Hit by a player or summon while it
// wants to act on someone, it hates the hitter by the damage, and when the
// hitter is the one it most wants to act on it may strike it with its
// self range physical special skill and its second debuff.
//
// Its memory: Creature0 is the armed player it saw last, itself until it
// sees one; IntAI3 is set once it shouted.
func PatrolWeaponEquipped() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22124, 22126, 22129}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.AddWanderDesire(5, 5)
			n.SetSlot(script.SlotCreature0, n)
			n.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			base.NoDesire(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if p, ok := e.Creature.(*script.Player); ok {
				if p.RightHandItemID() != 0 {
					addCastByType(n, p, npc.SkillTypeDebuff, 1000000)
					n.SetSlot(script.SlotCreature0, p)
				}
			}
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil {
					if script.Same(top, attacker) {
						if s.Rnd(100) < 33 {
							addCastByType(n, attacker, npc.SkillTypeSelfRangePhysicalSpecial, 1000000)
						}
						if s.Rnd(100) < 33 {
							addCastByType(n, attacker, npc.SkillTypeDebuff2, 1000000)
						}
					}
					// A hit of no damage counts as 1, for the parent too.
					if e.Damage == 0 {
						e.Damage = 1
					}
					n.AddAttackDesire(attacker, float64(e.Damage)/float64(n.Level()+7)*10000)
				}
			}
			base.Attacked(s, e)
		},
		// The skill finished hook reaches no parent.
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if isSkillOfType(n, e.Skill, npc.SkillTypeDebuff) {
				if n.Int(script.SlotIntAI3) == 0 {
					n.Say(script.NpcString(1121006))
					n.SetInt(script.SlotIntAI3, 1)
				}
				n.AddAttackDesire(n.Slot(script.SlotCreature0), 10000)
			}
		},
	})}
}
