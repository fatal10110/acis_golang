package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castDDMagicPhysicalSpecialDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastDDMagicPhysicalSpecial"

// CastDDMagicPhysicalSpecial is WarriorCastDDMagicPhysicalSpecial: a
// Warrior that casts its long range magic on the player or summon it hates
// most when that one hits it from farther than 200, and, on a draw under
// 33, its ranged physical special on a closer one it does not hate most,
// or on such an attacker of a clan member once it lived 7 s. Whenever one
// of its casts ends, it casts its long range magic on the player it hates
// most standing farther than 200.
func CastDDMagicPhysicalSpecial() script.Script {
	return script.Script{Dir: castDDMagicPhysicalSpecialDir, Behavior: true, NPCs: []int32{22046, 22049}, Hooks: castDDMagicPhysicalSpecialHooks()}
}

// castDDMagicPhysicalSpecialHooks are WarriorCastDDMagicPhysicalSpecial's
// hooks, which its children derive theirs from.
func castDDMagicPhysicalSpecialHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil {
					if n.Distance2D(attacker) > 200 && script.Same(mostHated, attacker) {
						addCastByType(n, attacker, npc.SkillTypeWLongRangeDDMagic, 1000000)
					}
					if s.Rnd(100) < 33 && !script.Same(mostHated, attacker) && n.Distance2D(attacker) < 200 {
						addCastByType(n, attacker, npc.SkillTypePhysicalSpecialRange, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() {
				if mostHated := called.MostHated(); mostHated != nil {
					if called.Distance2D(attacker) < 200 && s.Rnd(100) < 33 && !script.Same(mostHated, attacker) {
						addCastByType(called, attacker, npc.SkillTypePhysicalSpecialRange, 1000000)
					}
				}
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if mostHated := n.MostHated(); mostHated != nil {
				if n.Distance2D(mostHated) > 200 && isPlayer(mostHated) {
					addCastByType(n, mostHated, npc.SkillTypeWLongRangeDDMagic, 1000000)
				}
			}
			base.UseSkillFinished(s, e)
		},
	}).Named("WarriorCastDDMagicPhysicalSpecial")
}

// CastDDMagicPhysicalSpecialAggressive is
// WarriorCastDDMagicPhysicalSpecialAggressive: a
// WarriorCastDDMagicPhysicalSpecial that, once it lived 7 s, wanting no
// one and in its territory, attacks the players and summons it sees,
// casting its long range magic first on one farther than 200. A player or
// summon seen otherwise is ignored.
func CastDDMagicPhysicalSpecialAggressive() script.Script {
	base := castDDMagicPhysicalSpecialHooks()
	return script.Script{Dir: castDDMagicPhysicalSpecialDir, Behavior: true, NPCs: []int32{21308, 21392, 21389, 21429, 21393}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.TopDesireTarget() == nil && n.InMyTerritory() {
				if n.Distance2D(c) > 200 {
					addCastByType(n, c, npc.SkillTypeWLongRangeDDMagic, 1000000)
				}
				defaultnpc.TryToAttack(s, n, c)
				base.SeeCreature(s, e)
			}
		},
	})}
}

// CastDDMagicPhysicalSpecialBuffAggressive is
// WarriorCastDDMagicPhysicalSpecialBuffAggressive: a
// WarriorCastDDMagicPhysicalSpecial that buffs itself as it spawns and
// attacks the players and summons it sees. While it neither attacks nor
// casts it buffs itself again when it sees any creature that, not itself,
// holds no effect of the buff's stack, and when a player or summon hits it
// with the buff worn off; once it lived 7 s, a player's or summon's hit on
// a clan member makes it buff that member when the member holds none.
func CastDDMagicPhysicalSpecialBuffAggressive() script.Script {
	base := castDDMagicPhysicalSpecialHooks()
	return script.Script{Dir: castDDMagicPhysicalSpecialDir, Behavior: true, NPCs: []int32{21384, 21643}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			addCastByType(e.NPC, e.NPC, npc.SkillTypeBuff, 1000000)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !fighting(n.Intention()) {
				buff, ok := n.SkillByType(npc.SkillTypeBuff)
				if s.AbnormalLevel(c, buff) <= 0 && ok {
					n.AddCastDesire(n, buff, 1000000)
				}
			}
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Playable() {
				if !fighting(n.Intention()) {
					buff, ok := n.SkillByType(npc.SkillTypeBuff)
					if s.AbnormalLevel(n, buff) <= 0 && ok {
						n.AddCastDesire(n, buff, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 {
				if !fighting(called.Intention()) {
					buff, ok := called.SkillByType(npc.SkillTypeBuff)
					if s.AbnormalLevel(caller, buff) <= 0 && ok {
						called.AddCastDesire(caller, buff, 1000000)
					}
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}
