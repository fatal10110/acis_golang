package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PhysicalSpecialPotionAggressive is
// WarriorPhysicalSpecialPotionAggressive: a WarriorPhysicalSpecialAggressive
// that, hit below 0.8, 0.5 or 0.33 of its hit points, may heal itself,
// until it has healed below a third, and may strike its attacker with its
// physical special skill; each heal cast ends in one of three lines.
//
// Its memory: IntAI1 is the last band it healed in, 3 for the lowest.
func PhysicalSpecialPotionAggressive() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22022, 22023, 22026}, Hooks: base.With(script.Hooks{
		// The created hook reaches no parent.
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			switch {
			case n.HPRatio() < 0.8 && n.Int(script.SlotIntAI1) != 3 && s.Rnd(100) < 33:
				addCastByType(n, n, npc.SkillTypeMagicHeal, 1000000)
				n.SetInt(script.SlotIntAI1, 1)
			case n.HPRatio() < 0.5 && n.Int(script.SlotIntAI1) != 3 && s.Rnd(100) < 33:
				addCastByType(n, n, npc.SkillTypeMagicHeal, 1000000)
				n.SetInt(script.SlotIntAI1, 2)
			case n.HPRatio() < 0.33 && n.Int(script.SlotIntAI1) != 3 && s.Rnd(100) < 33:
				addCastByType(n, n, npc.SkillTypeMagicHeal, 1000000)
				n.SetInt(script.SlotIntAI1, 3)
			}
			if s.Rnd(100) < 33 {
				castPhysicalSpecial(n, e.Attacker)
			}
			base.Attacked(s, e)
		},
		// The skill finished hook reaches no parent.
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			i0 := s.Rnd(100)
			if isSkillOfType(n, e.Skill, npc.SkillTypeMagicHeal) {
				switch {
				case i0 < 30:
					n.Say(script.NpcString(10071))
				case i0 < 60:
					n.Say(script.NpcString(10072))
				default:
					n.Say(script.NpcString(10073))
				}
			}
		},
	})}
}

// PhysicalSpecialSpoilAggressive is WarriorPhysicalSpecialSpoilAggressive:
// a WarriorPhysicalSpecialAggressive that, first hit, wants its effect
// skill on the attacker, saying one of three lines when it lands, may
// strike the attacker with its physical special skill on any hit, and
// gloats over a player its attack killed.
//
// Its memory: IntAI1 is set once it wanted its effect skill.
func PhysicalSpecialSpoilAggressive() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22017, 22024}, Hooks: base.With(script.Hooks{
		// The created hook reaches no parent.
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI1) == 0 {
				addCastByType(n, e.Attacker, npc.SkillTypeEffectSkill, 1000000)
				n.SetInt(script.SlotIntAI1, 1)
			}
			if s.Rnd(100) < 33 {
				castPhysicalSpecial(n, e.Attacker)
			}
			base.Attacked(s, e)
		},
		// The attack finished hook reaches no parent.
		OnAttackFinished: func(_ *script.Script, e script.AttackFinished) {
			if _, ok := e.Target.(*script.Player); ok && e.Target.Dead() {
				e.NPC.Say(script.NpcString(1010584))
			}
		},
		// The skill finished hook reaches no parent.
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if isSkillOfType(n, e.Skill, npc.SkillTypeEffectSkill) && e.Success {
				switch i0 := s.Rnd(100); {
				case i0 < 30:
					n.Say(script.NpcString(10068))
				case i0 < 60:
					n.Say(script.NpcString(10069))
				default:
					n.Say(script.NpcString(10070))
				}
			}
		},
	})}
}
