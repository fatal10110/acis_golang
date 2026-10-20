package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const physicalSpecialDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorPhysicalSpecial"

// RoyalRushWarriorPhysicalSpecial is RoyalRushWarriorPhysicalSpecial: a
// WarriorPhysicalSpecial of the Four Sepulchers that keeps its self buff up
// every two minutes, strikes its top desire target close by with its
// physical special skill and, below half of its hit points, draws its
// weapon (AI parameter WeaponID) glowing at +15 and stops striking. A
// script event 1234 names a creature for it to attack.
//
// Its memory: IntAI0 is set once it drew its weapon.
func RoyalRushWarriorPhysicalSpecial() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{18133, 18136, 18148, 18166, 18169, 18187, 18190, 18222, 18225, 21418}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if selfBuff, ok := n.SkillByType(npc.SkillTypeSelfBuff); ok {
				n.AddCastDesire(n, selfBuff, 1000000)
				s.StartTimerAtFixedRate("3000", n, nil, 120000*time.Millisecond, 120000*time.Millisecond)
			}
			n.SetInt(script.SlotIntAI0, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			weaponID := n.IntParam("WeaponID")
			if n.HPRatio() < 0.5 && weaponID != 0 {
				n.EquipItem(weaponID, 0)
				n.SetEnchantEffect(15)
				n.SetInt(script.SlotIntAI0, 1)
			}
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) && n.Distance2D(attacker) < 100 {
						if n.Int(script.SlotIntAI0) == 0 {
							castPhysicalSpecial(n, attacker)
						} else if s.Rnd(100) < 20 {
							n.RechargeShots(true, false)
						}
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 && called.Distance2D(attacker) > 100 {
					if called.Int(script.SlotIntAI0) == 0 {
						castPhysicalSpecial(called, attacker)
					} else if s.Rnd(100) < 20 {
						called.RechargeShots(true, false)
					}
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3000") {
				n := e.NPC
				if selfBuff, ok := n.SkillByType(npc.SkillTypeSelfBuff); ok && s.AbnormalLevel(n, selfBuff) <= 0 {
					n.AddCastDesire(n, selfBuff, 1000000)
				}
			}
			return base.Timer(s, e)
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

// RoyalRushWarriorStone is RoyalRushWarriorStone: a WarriorPhysicalSpecial
// of the Four Sepulchers that buffs itself when created, attacks the
// players and summons it sees while settled and not attacking, and, hit by
// its top desire target, may debuff it, blast around itself or buff itself
// again; the second buff may also answer a hit on its clan.
func RoyalRushWarriorStone() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{18243}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			addCastByType(e.NPC, e.NPC, npc.SkillTypeSelfBuff1, 1000000)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack {
				if !c.Playable() {
					return
				}
				defaultnpc.TryToAttack(s, n, c)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, attacker, npc.SkillTypeDebuff, 1000000)
					}
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, n, npc.SkillTypeSelfRangeDDMagic, 1000000)
					}
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, n, npc.SkillTypeSelfBuff2, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if called.LifeTime() > 7 && e.Attacker.Playable() && called.Intention() != script.IntentionAttack && s.Rnd(100) < 33 {
				addCastByType(called, called, npc.SkillTypeSelfBuff2, 1000000)
			}
			base.ClanAttacked(s, e)
		},
	})}
}
