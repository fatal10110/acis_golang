package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castSplashDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastSplash"

// CastSplash is WarriorCastSplash: a Warrior that, on a draw under 33,
// casts its ranged DD magic around itself when the player or summon it
// hates most hits it, or, once it lived 7 s, a clan member, from closer
// than 200.
func CastSplash() script.Script {
	return script.Script{Dir: castSplashDir, Behavior: true, NPCs: []int32{20449, 20483, 21287, 21114, 21107, 21651, 21284, 21291}, Hooks: castSplashHooks()}
}

// castSplashHooks are WarriorCastSplash's hooks, which its children derive
// theirs from.
func castSplashHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) && n.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
					addCastByType(n, n, npc.SkillTypeSelfRangeDDMagic, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if script.Same(called.MostHated(), attacker) && called.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
					addCastByType(called, called, npc.SkillTypeSelfRangeDDMagic, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCastSplash")
}

// CastSplashAggressive is WarriorCastSplashAggressive: a WarriorCastSplash
// that attacks the players and summons it sees.
func CastSplashAggressive() script.Script {
	base := castSplashHooks()
	return script.Script{Dir: castSplashDir, Behavior: true, NPCs: []int32{21428, 20815, 20212, 20294, 20512, 20549, 21141, 21147, 21149, 21153, 21159, 21164}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}

// Agit02DoomTrooperAgit is Agit02DoomTrooperAgit: a clan hall siege
// trooper, a WarriorCastSplash that never wanders and answers only for
// itself and the clan members sharing its flag. The player or summon it
// most wants to attack hitting it, or such a member once it lived 7 s,
// from closer than 200 may draw its ranged DD magic, around itself when
// hit, on the attacker for a member; every such hit, and every spell it
// sees, makes it hold its ground to attack by the damage or the spell's
// hate dealt; a player or summon it sees, by 200.
func Agit02DoomTrooperAgit() script.Script {
	base := castSplashHooks()
	return script.Script{Dir: castSplashDir, Behavior: true, NPCs: []int32{35414, 35635}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotFlag, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				topDesireTarget := n.TopDesireTarget()
				selfRangeDDMagic, ok := n.SkillByType(npc.SkillTypeSelfRangeDDMagic)
				if topDesireTarget != nil && n.Distance2D(attacker) < 200 && s.Rnd(33) < 10 && script.Same(topDesireTarget, attacker) && ok {
					n.AddCastDesire(n, selfRangeDDMagic, 1000000)
				}
			}
			if attacker.Playable() {
				n.AddAttackDesireHold(attacker, float64(e.Damage)/float64(n.MaxHP())/0.05*100)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			sameFlag := called.Int(script.SlotFlag) == caller.Int(script.SlotFlag)
			if called.LifeTime() > 7 && attacker.Playable() && sameFlag {
				topDesireTarget := called.TopDesireTarget()
				selfRangeDDMagic, ok := called.SkillByType(npc.SkillTypeSelfRangeDDMagic)
				if topDesireTarget != nil && called.Distance2D(attacker) < 200 && s.Rnd(33) < 10 && script.Same(topDesireTarget, attacker) && ok {
					called.AddCastDesire(attacker, selfRangeDDMagic, 1000000)
				}
			}
			if sameFlag && attacker.Playable() {
				called.AddAttackDesireHold(attacker, float64(e.Damage)/float64(called.MaxHP())/0.05*100)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesireHold(e.Caster, float64(aggro)/float64(e.NPC.MaxHP())/0.05*150)
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddAttackDesireHold(e.Creature, 200)
		},
	})}
}
