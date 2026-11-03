package warrior

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// cast3Magical2Dir is the directory of WarriorCast3SkillsMagical2 and its
// children.
const cast3Magical2Dir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCast3SkillsMagical2"

// Cast3SkillsMagical2 is WarriorCast3SkillsMagical2: a Warrior that, hit
// from more than 100 away by the player or summon it hates most, may
// blast it with either of its two DD magics and lay its debuff on it
// unless it already holds one; a settled clan member not attacking does
// the same to its clan's attacker, the blasts only from more than 100.
func Cast3SkillsMagical2() script.Script {
	return script.Script{Dir: cast3Magical2Dir, Behavior: true, NPCs: []int32{20411, 20416, 20505, 20618, 20862, 27184, 21396, 20202, 21020, 21108, 20154, 20223, 20225, 20799, 20109, 20609, 20415, 21595}, Hooks: Cast3SkillsMagical2Hooks()}
}

// Cast3SkillsMagical2Hooks are WarriorCast3SkillsMagical2's hooks, which
// its children derive theirs from.
func Cast3SkillsMagical2Hooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Distance2D(attacker) > 100 {
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, attacker) {
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic1)
					}
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic2)
					}
					debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
						castByType(n, attacker, npc.SkillTypeDebuff)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack {
				if called.Distance2D(attacker) > 100 {
					if s.Rnd(100) < 33 {
						castByType(called, attacker, npc.SkillTypeDDMagic1)
					}
					if s.Rnd(100) < 33 {
						castByType(called, attacker, npc.SkillTypeDDMagic2)
					}
				}
				debuff, _ := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
					castByType(called, attacker, npc.SkillTypeDebuff)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCast3SkillsMagical2")
}

// Cast3SkillsMagical2Aggressive is WarriorCast3SkillsMagical2Aggressive: a
// WarriorCast3SkillsMagical2 that attacks the players and summons it sees;
// settled in its territory it may first blast one more than 100 away with
// either DD magic and, while wandering, lay its debuff on one holding none.
func Cast3SkillsMagical2Aggressive() script.Script {
	base := Cast3SkillsMagical2Hooks()
	return script.Script{Dir: cast3Magical2Dir, Behavior: true, NPCs: []int32{18001, 20621, 21006, 20825, 20160, 20198, 21066, 20612, 21647, 21026, 20056, 20117, 20118, 20352, 20421, 18002, 22119}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() {
				if n.Distance2D(c) > 100 {
					if s.Rnd(100) < 33 {
						castByType(n, c, npc.SkillTypeDDMagic1)
					}
					if s.Rnd(100) < 33 {
						castByType(n, c, npc.SkillTypeDDMagic2)
					}
				}
				if n.Intention() == script.IntentionWander {
					debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 {
						castByType(n, c, npc.SkillTypeDebuff)
					}
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}

// Cast3SkillsMagical2Revived is WarriorCast3SkillsMagical2Revived: a
// WarriorCast3SkillsMagical2 that, called with spawn parameters 1000 and
// a creature, stuns and attacks that creature as it appears.
func Cast3SkillsMagical2Revived() script.Script {
	base := Cast3SkillsMagical2Hooks()
	return script.Script{Dir: cast3Magical2Dir, Behavior: true, NPCs: []int32{20860, 21071}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.Int(script.SlotParam1) == 1000 {
				if c0 := s.Object(n.Int(script.SlotParam2)); c0 != nil {
					n.AddCastDesire(c0, skill.Ref{ID: 4663, Level: 1}, 10000)
					n.AddAttackDesire(c0, 500)
				}
			}
			base.Created(s, e)
		},
	})}
}

// Cast3SkillsMagical2StoneAggressive is
// WarriorCast3SkillsMagical2StoneAggressive: a Ketra or Varka
// WarriorCast3SkillsMagical2 that attacks the players and summons it sees
// unless allied with its own faction, and petrifies such an ally who
// attacks it or its clan, or who helps another.
func Cast3SkillsMagical2StoneAggressive() script.Script {
	base := Cast3SkillsMagical2Hooks()
	return script.Script{Dir: cast3Magical2Dir, Behavior: true, NPCs: []int32{21331, 21357}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil {
				return
			}
			if p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan") {
				return
			}
			if p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan") {
				return
			}
			defaultnpc.TryToAttack(s, n, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if defaultnpc.MaybeCastPetrify(e.NPC, e.Attacker) {
				return
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
