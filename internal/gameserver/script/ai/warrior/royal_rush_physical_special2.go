package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// RoyalRushWarriorPhysicalSpecial2 is RoyalRushWarriorPhysicalSpecial2: a
// Warrior of the Four Sepulchers that, below half of its hit points, draws
// its weapon (AI parameter WeaponID) glowing at +15 and from then on charges
// soulshots instead of striking close by with its second physical special
// skill. A script event 1234 names a creature for it to attack.
//
// Its memory: IntAI0 is set once it drew its weapon.
func RoyalRushWarriorPhysicalSpecial2() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{18135, 18146, 18168, 18189, 18224, 21419}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			weaponID := n.IntParam("WeaponID")
			if n.HPRatio() < 0.5 && weaponID != 0 {
				n.EquipItem(weaponID, 0)
				n.SetEnchantEffect(15)
				n.SetInt(script.SlotIntAI0, 1)
			}
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) && n.Distance2D(attacker) < 100 && n.Int(script.SlotIntAI0) == 0 {
						addCastByType(n, attacker, npc.SkillTypePhysicalSpecial2, 1000000)
					}
					if s.Rnd(100) < 33 && n.Int(script.SlotIntAI0) == 1 {
						n.RechargeShots(true, false)
					}
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) && n.Distance2D(attacker) > 100 {
						addCastByType(n, attacker, npc.SkillTypePhysicalSpecial2, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 && called.Distance2D(attacker) > 100 {
					addCastByType(called, attacker, npc.SkillTypePhysicalSpecial1, 1000000)
				}
				if s.Rnd(100) < 33 && called.Distance2D(attacker) < 100 {
					if called.Int(script.SlotIntAI0) == 0 {
						addCastByType(called, attacker, npc.SkillTypePhysicalSpecial2, 1000000)
					} else {
						called.RechargeShots(true, false)
					}
				}
			}
			base.ClanAttacked(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == 1234 {
				if c0 := s.Object(e.Arg1); c0 != nil {
					if s.Rnd(100) < 80 {
						e.NPC.AddAttackDesire(c0, 300)
					} else {
						e.NPC.RemoveAllAttackDesire()
						e.NPC.AddAttackDesire(c0, 1000)
					}
				}
			}
		},
	})}
}
