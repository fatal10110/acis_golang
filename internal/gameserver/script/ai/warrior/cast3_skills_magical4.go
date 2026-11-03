package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// cast3Magical4Dir is the directory of WarriorCast3SkillsMagical4 and its
// children.
const cast3Magical4Dir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCast3SkillsMagical4"

// Cast3SkillsMagical4 is WarriorCast3SkillsMagical4: a Warrior that, hit
// below 70% of its HP, may heal itself. Hit by a player or summon, until
// one first hits it (IntAI1) it may buff itself above half its HP; else,
// once (IntAI2), above 60% and hating someone, it may cancel the attacker
// unless the attacker holds one of its marks. A clan member not attacking
// may heal the member its clan's attacker hit.
func Cast3SkillsMagical4() script.Script {
	return script.Script{Dir: cast3Magical4Dir, Behavior: true, NPCs: []int32{21197, 21200, 21203}, Hooks: Cast3SkillsMagical4Hooks()}
}

// Cast3SkillsMagical4Hooks are WarriorCast3SkillsMagical4's hooks, which
// its children derive theirs from.
func Cast3SkillsMagical4Hooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			hpRatio := n.HPRatio()
			if s.Rnd(100) < 33 && hpRatio < 0.7 {
				castByType(n, n, npc.SkillTypeMagicHeal)
			}
			if attacker.Playable() {
				if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && hpRatio > 0.5 {
					castByType(n, n, npc.SkillTypeBuff)
				} else if s.Rnd(200) < 1 && n.Int(script.SlotIntAI2) == 0 && hpRatio > 0.6 {
					if mostHated := n.MostHated(); mostHated != nil {
						if unmarked(s, n, attacker, npc.SkillTypeCheckMagic, npc.SkillTypeCheckMagic1, npc.SkillTypeCheckMagic2) {
							castByType(n, attacker, npc.SkillTypeCancelMagic)
							n.SetInt(script.SlotIntAI2, 1)
						}
					}
				}
				n.SetInt(script.SlotIntAI1, 1)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if s.Rnd(100) < 33 && called.Intention() != script.IntentionAttack {
				castByType(called, e.Caller, npc.SkillTypeMagicHeal)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCast3SkillsMagical4")
}

// Cast3SkillsMagical4Revived is WarriorCast3SkillsMagical4Revived: a
// WarriorCast3SkillsMagical4 that, called with spawn parameters 1000 and
// a creature, stuns and attacks that creature as it appears.
func Cast3SkillsMagical4Revived() script.Script {
	base := Cast3SkillsMagical4Hooks()
	return script.Script{Dir: cast3Magical4Dir, Behavior: true, NPCs: []int32{21206}, Hooks: base.With(script.Hooks{
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
