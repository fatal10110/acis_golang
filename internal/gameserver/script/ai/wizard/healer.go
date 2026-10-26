package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Healer is WizardHealer: a WizardDDMagic2 that, created, wants its buff on
// itself, without its parent's timers. Hit above half its HP, on a draw
// under 33, it runs 500 from the attacker instead of answering, until it
// gets stuck running from its top desire target with some HP lost (int
// slot 2). A clan member's call above half that member's HP may buff the
// member and debuff the attacker, on a draw under 33; at half or less it
// may heal the member, on a draw under 50. When a clan member dies it runs
// 500 from the killer.
func Healer() script.Script {
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21300, 21305}, Hooks: healerHooks()}
}

// healerHooks are Healer's hooks, which its child derives its from.
func healerHooks() script.Hooks {
	base := ddMagic2Hooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			n := e.NPC
			castByType(n, n, npc.SkillTypeBuff, 1000000)
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI2, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			hpRatio := n.HPRatio()
			if s.Rnd(100) < 33 && hpRatio > 0.5 {
				if n.Int(script.SlotIntAI2) == 0 {
					n.AddFleeDesire(attacker, 500, 1000000)
					if n.GeoPathFailCount() > 3 && script.Same(attacker, n.TopDesireTarget()) && hpRatio != 1.0 {
						n.SetInt(script.SlotIntAI2, 1)
					}
				} else {
					base.Attacked(s, e)
				}
			} else {
				base.Attacked(s, e)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			i0 := s.Rnd(100)
			if caller.HPRatio() > 0.5 {
				if i0 < 33 {
					castByType(called, caller, npc.SkillTypeBuff, 1000000)
					castByType(called, e.Attacker, npc.SkillTypeDebuff, 1000000)
				}
			} else if i0 < 50 {
				castByType(called, caller, npc.SkillTypeHeal, 1000000)
			}
			base.ClanAttacked(s, e)
		},
		OnClanDied: func(_ *script.Script, e script.ClanDied) {
			e.Called.AddFleeDesire(e.Killer, 500, 1000000)
		},
	}).Named("WizardHealer")
}

// HealerAggressive is WizardHealerAggressive: a WizardHealer that tries to
// attack a playable it sees and ignores anything else it sees.
func HealerAggressive() script.Script {
	base := healerHooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21412}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}
