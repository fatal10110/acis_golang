package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castEnchantPhysicalPoisonDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastEnchantPhysicalPoison"

// CastEnchantPhysicalPoison is WarriorCastEnchantPhysicalPoison: a Warrior
// that casts both its buffs on itself as it spawns, and again, each worn
// off, when hit; a hit may draw its physical special on the attacker. A
// hit on it or a clan member, or a spell it sees, may draw either debuff
// on the attacker or caster, a deeper level of it while the target holds a
// shallow one. It binds no NPC of its own.
func CastEnchantPhysicalPoison() script.Script {
	return script.Script{Dir: castEnchantPhysicalPoisonDir, Behavior: true, Hooks: castEnchantPhysicalPoisonHooks()}
}

// castEnchantPhysicalPoisonHooks are WarriorCastEnchantPhysicalPoison's
// hooks, which its children derive theirs from.
func castEnchantPhysicalPoisonHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			addCastByType(n, n, npc.SkillTypeBuff1, 1000000)
			addCastByType(n, n, npc.SkillTypeBuff2, 1000000)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if buff1, ok := n.SkillByType(npc.SkillTypeBuff1); s.AbnormalLevel(n, buff1) <= 0 && ok {
				n.AddCastDesire(n, buff1, 1000000)
			}
			if buff2, ok := n.SkillByType(npc.SkillTypeBuff2); s.AbnormalLevel(n, buff2) <= 0 && ok {
				n.AddCastDesire(n, buff2, 1000000)
			}
			if s.Rnd(100) < 33 {
				addCastByType(n, attacker, npc.SkillTypePhysicalSpecial, 1000000)
			}
			maybeCastDebuffs(s, attacker, n)
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			maybeCastDebuffs(s, e.Attacker, e.Called)
			base.ClanAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			maybeCastDebuffs(s, e.Caster, e.NPC)
			base.SeeSpell(s, e)
		},
	}).Named("WarriorCastEnchantPhysicalPoison")
}

// maybeCastDebuffs makes n want, on each of two draws under 10, one of its
// two debuffs on attacker, weight 1000000: the template's level while
// attacker holds no effect of its stack (the second also at stack level
// 0), else that level raised by the one attacker holds, below level 10.
func maybeCastDebuffs(s *script.Script, attacker script.Creature, n *script.NPC) {
	if s.Rnd(100) < 10 {
		debuff1, ok := n.SkillByType(npc.SkillTypeDebuff1)
		if i1 := s.AbnormalLevel(attacker, debuff1); i1 <= 0 {
			if ok {
				n.AddCastDesire(attacker, debuff1, 1000000)
			}
		} else if i1 < 10 {
			n.AddCastDesire(attacker, skill.Ref{ID: debuff1.ID, Level: debuff1.Level + int(i1)}, 1000000)
		}
	}
	if s.Rnd(100) < 10 {
		debuff2, ok := n.SkillByType(npc.SkillTypeDebuff2)
		if i1 := s.AbnormalLevel(attacker, debuff2); i1 <= -1 {
			if ok {
				n.AddCastDesire(attacker, debuff2, 1000000)
			}
		} else if i1 < 10 {
			n.AddCastDesire(attacker, skill.Ref{ID: debuff2.ID, Level: debuff2.Level + int(i1)}, 1000000)
		}
	}
}

// castEnchantPhysicalPoisonAggressiveHooks are
// WarriorCastEnchantPhysicalPoisonAggressive's hooks, which its children
// derive theirs from.
func castEnchantPhysicalPoisonAggressiveHooks() script.Hooks {
	base := castEnchantPhysicalPoisonHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorCastEnchantPhysicalPoisonAggressive")
}

// CastEnchantPhysicalPoisonAggressive is
// WarriorCastEnchantPhysicalPoisonAggressive: a
// WarriorCastEnchantPhysicalPoison that attacks the players and summons it
// sees.
func CastEnchantPhysicalPoisonAggressive() script.Script {
	return script.Script{Dir: castEnchantPhysicalPoisonDir, Behavior: true, NPCs: []int32{21316}, Hooks: castEnchantPhysicalPoisonAggressiveHooks()}
}

// CastDDMagicEnchantPhysicalPoisonAggressive is
// WarriorCastDDMagicEnchantPhysicalPoisonAggressive: a
// WarriorCastEnchantPhysicalPoisonAggressive that, on a draw under 33,
// casts its DD magic on the player or summon it hates most hitting it from
// farther than 100, and, once it lived 7 s and while not attacking, on
// such a player or summon hitting a clan member.
func CastDDMagicEnchantPhysicalPoisonAggressive() script.Script {
	base := castEnchantPhysicalPoisonAggressiveHooks()
	return script.Script{Dir: castEnchantPhysicalPoisonDir, Behavior: true, NPCs: []int32{21321, 21317, 21322, 21314}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if n.Distance2D(attacker) > 100 {
					if mostHated := n.MostHated(); mostHated != nil {
						if script.Same(mostHated, attacker) && s.Rnd(100) < 33 {
							addCastByType(n, attacker, npc.SkillTypeDDMagic, 1000000)
						}
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack {
				if called.Distance2D(attacker) > 100 && s.Rnd(100) < 33 {
					addCastByType(called, attacker, npc.SkillTypeDDMagic, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// CastEnchantRangePhysicalPoisonAggressive is
// WarriorCastEnchantRangePhysicalPoisonAggressive: a
// WarriorCastEnchantPhysicalPoisonAggressive that, on a draw under 33,
// answers a player's or summon's hit, not by the one it hates most, with
// its ranged physical special, else its physical special, whichever it can
// cast first; able to cast neither, it marks IntAI0 and attacks the
// attacker with weight 1000.
func CastEnchantRangePhysicalPoisonAggressive() script.Script {
	base := castEnchantPhysicalPoisonAggressiveHooks()
	return script.Script{Dir: castEnchantPhysicalPoisonDir, Behavior: true, NPCs: []int32{21319}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil {
					if s.Rnd(100) < 33 && !script.Same(mostHated, attacker) {
						rangePhysicalSpecial, _ := n.SkillByType(npc.SkillTypeRangePhysicalSpecial)
						physicalSpecial, _ := n.SkillByType(npc.SkillTypePhysicalSpecial)
						if n.MeetsHPMPDisabledConditions(attacker, rangePhysicalSpecial) {
							n.AddCastDesire(attacker, rangePhysicalSpecial, 1000000)
						} else if n.MeetsHPMPDisabledConditions(attacker, physicalSpecial) {
							n.AddCastDesire(attacker, physicalSpecial, 1000000)
						} else {
							n.SetInt(script.SlotIntAI0, 1)
							n.AddAttackDesire(attacker, 1000)
						}
					}
				}
			}
			base.Attacked(s, e)
		},
	})}
}
