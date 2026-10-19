package monster

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// lv3Dir is the directory the LV3 monsters are listed under.
const lv3Dir = "ai/individual/Monster/LV3Monster"

// LV3Monster is a monster a quest raises for one player, the one its
// second spawn parameter names. As it spawns it attacks the creature its
// first parameter names and remembers its master when its third parameter
// names an NPC. A hit or a spell from any other player, or 12 minutes
// passing, makes it leave the world, first telling its master (event
// 1000) and clearing the quest slot of the NPC its third parameter names.
// It binds no NPC of its own.
func LV3Monster() script.Script {
	return script.Script{Dir: lv3Dir, Behavior: true, Hooks: lv3MonsterHooks()}
}

// lv3MonsterHooks are LV3Monster's hooks, which its children derive theirs
// from.
func lv3MonsterHooks() script.Hooks {
	base := Hooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.Int(script.SlotParam1) != 0 {
				n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
				if c0 := n.Slot(script.SlotCreature0); c0 != nil {
					n.AddAttackDesire(c0, 200)
				}
			}
			if n.Int(script.SlotParam3) != 0 && n.HasMaster() {
				n.SetSlot(script.SlotCreature1, n.Master())
			}
			s.StartTimer("3000", n, nil, 12*60*1000*time.Millisecond)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if c0 := script.ActingPlayer(attacker); c0 != nil {
				if c0.ObjectID() != n.Int(script.SlotParam2) {
					lv3Leave(s, n)
				}
				if attacker.Playable() {
					f0 := defaultnpc.HateRatio(n, attacker)
					f0 = ((1.0 * float64(e.Damage)) / float64(n.Level()+7)) + ((f0 / 100) * ((1.0 * float64(e.Damage)) / float64(n.Level()+7)))
					n.AddAttackDesire(attacker, f0*100)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "3000") {
				clearQuestSlot(s, n)
				n.DeleteMe()
			}
			return ""
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if e.Caster != nil && e.Caster.ObjectID() != e.NPC.Int(script.SlotParam2) {
				lv3Leave(s, e.NPC)
			}
		},
	}).Named("LV3Monster")
}

// lv3Leave tells the master the NPC remembers that it leaves (event 1000),
// clears the quest slot of the NPC its third spawn parameter names and
// takes it out of the world.
func lv3Leave(s *script.Script, n *script.NPC) {
	if c1 := n.Slot(script.SlotCreature1); c1 != nil {
		s.SendScriptEvent(c1.(*script.NPC), 1000, 0, 0)
	}
	clearQuestSlot(s, n)
	n.DeleteMe()
}

// clearQuestSlot zeroes quest slot 0 of the NPC n's third spawn parameter
// names, when it is in the world.
func clearQuestSlot(s *script.Script, n *script.NPC) {
	if npc0, _ := s.Object(n.Int(script.SlotParam3)).(*script.NPC); npc0 != nil {
		npc0.SetInt(npc.IntQuest0, 0)
	}
}

// lv3Strike adds the LV3 attacked formula of a playable attacker to the
// NPC's attack desire, a damage of 0 counting as 1, and, when attacker is
// its top desire target, a 33% chance to want to strike it with one of its
// three physical specials. It returns the damage the parent call passes on.
func lv3Strike(s *script.Script, n *script.NPC, attacker script.Creature, damage int32) int32 {
	if attacker.Playable() {
		if damage == 0 {
			damage = 1
		}
		n.AddAttackDesire(attacker, ((1.0*float64(damage))/float64(n.Level()+7))*100)
		if script.Same(n.TopDesireTarget(), attacker) && s.Rnd(100) < 33 {
			switch s.Rnd(3) {
			case 0:
				castByType(n, attacker, npc.SkillTypePhysicalSpecial1, 1000)
			case 1:
				castByType(n, attacker, npc.SkillTypePhysicalSpecial2, 1000)
			case 2:
				castByType(n, attacker, npc.SkillTypePhysicalSpecial3, 1000)
			}
		}
	}
	return damage
}

// castByType makes n want to cast at target, with weight, the skill its
// template lists under t; a template that lists none casts nothing.
func castByType(n *script.NPC, target script.Creature, t npc.SkillType, weight float64) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, weight)
	}
}

// LV3Dagger is an LV3Monster that buffs the creature its first spawn
// parameter names as it spawns, strikes its top desire target with its
// physical specials and every 15 s may switch its weak mode.
func LV3Dagger() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27299, 27300, 27301}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("4000", n, nil, 15000*time.Millisecond, 15000*time.Millisecond)
			if n.Int(script.SlotParam1) != 0 {
				n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
				if c0 := n.Slot(script.SlotCreature0); c0 != nil {
					castByType(n, c0, npc.SkillTypeBuff, 1000000)
				}
			}
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			e.Damage = lv3Strike(s, e.NPC, e.Attacker, e.Damage)
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "4000") {
				if s.Rnd(100) < 33 {
					if s.Rnd(100) < 60 {
						castByType(n, n, npc.SkillTypeWeakModeFalse, 1000000)
					} else {
						castByType(n, n, npc.SkillTypeWeakModeTrue, 1000000)
					}
				}
			}
			return base.Timer(s, e)
		},
	})}
}

// LV3Knight is an LV3Monster that raises its defence mode as it spawns and
// for 5 to 10 s after a hit by skill 28 or 18, strikes its top desire
// target with its physical specials, and once under half its HP may go
// into its crazy mode for 5 to 10 s.
func LV3Knight() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27258, 27271, 27286, 27287}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			n.SetInt(script.SlotIntAI1, 0)
			castByType(n, n, npc.SkillTypeDefenceMode, 1000000)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if (e.Skill.ID == 28 || e.Skill.ID == 18) && n.Int(script.SlotIntAI0) == 0 {
				n.SetInt(script.SlotIntAI0, 1)
				castByType(n, n, npc.SkillTypeDefenceMode, 1000000)
				s.StartTimer("4000", n, nil, time.Duration(5000+(1000*s.Rnd(6)))*time.Millisecond)
			}
			e.Damage = lv3Strike(s, n, e.Attacker, e.Damage)
			if n.Int(script.SlotIntAI0) == 1 && n.Int(script.SlotIntAI1) == 0 && n.HPRatio() < 0.5 && s.Rnd(100) < 80 {
				n.SetInt(script.SlotIntAI0, 2)
				n.SetInt(script.SlotIntAI1, 1)
				castByType(n, n, npc.SkillTypeCrazyMode, 1000000)
				s.StartTimer("4001", n, nil, time.Duration(5000+(1000*s.Rnd(6)))*time.Millisecond)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "4000") {
				if n.Int(script.SlotIntAI0) == 1 {
					n.SetInt(script.SlotIntAI0, 0)
					castByType(n, n, npc.SkillTypeDefenceMode, 1000000)
				}
			} else if strings.EqualFold(e.Name, "4001") {
				if n.Int(script.SlotIntAI0) == 2 {
					n.SetInt(script.SlotIntAI0, 0)
					castByType(n, n, npc.SkillTypeDefenceMode, 1000000)
					castByType(n, n, npc.SkillTypeCrazyMode, 1000000)
				}
			}
			return base.Timer(s, e)
		},
	})}
}

// LV3Orc is an LV3Monster that spawns in its big body with a glowing
// weapon. Stunned and hit by skill 1245, it steps through its weakening:
// each step it reaches, checked every second while not stunned, drops its
// desires, casts the step's debuff on itself (the second also returns it
// to its normal body) and attacks the creature its first spawn parameter
// named.
func LV3Orc() script.Script {
	base := lv3MonsterHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27294, 27295}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			n.SetEnchantEffect(10)
			castByType(n, n, npc.SkillTypeBigBodySkill, 1000000)
			s.StartTimerAtFixedRate("4000", n, nil, 1000*time.Millisecond, 1000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "4000") {
				if !n.Stunned() {
					switch n.Int(script.SlotIntAI0) {
					case 1:
						n.RemoveAllDesire()
						castByType(n, n, npc.SkillTypeDebuff1, 1000000)
						n.SetInt(script.SlotIntAI0, n.Int(script.SlotIntAI0)+1)
						n.AddAttackDesire(n.Slot(script.SlotCreature0), 200)
					case 3:
						n.RemoveAllDesire()
						castByType(n, n, npc.SkillTypeDebuff2, 1000000)
						castByType(n, n, npc.SkillTypeNormalBodySkill, 1000000)
						n.SetInt(script.SlotIntAI0, n.Int(script.SlotIntAI0)+1)
						n.AddAttackDesire(n.Slot(script.SlotCreature0), 200)
					case 5:
						n.RemoveAllDesire()
						castByType(n, n, npc.SkillTypeDebuff3, 1000000)
						n.SetInt(script.SlotIntAI0, n.Int(script.SlotIntAI0)+1)
						n.AddAttackDesireDamage(n.Slot(script.SlotCreature0), 1, 200)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Stunned() && e.Skill.ID == 1245 {
				switch n.Int(script.SlotIntAI0) {
				case 0:
					n.SetInt(script.SlotIntAI0, n.Int(script.SlotIntAI0)+1)
					n.SetEnchantEffect(0)
				case 2, 4:
					n.SetInt(script.SlotIntAI0, n.Int(script.SlotIntAI0)+1)
				}
			}
			e.Damage = lv3Strike(s, n, e.Attacker, e.Damage)
			base.Attacked(s, e)
		},
	})}
}
