package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// SeeSpellAggressive is WarriorSeeSpellAggressive: a WarriorAggressive
// that keeps its self buff up, strikes the player it hates most with its
// physical special skill, and answers its clan only when a skill hit it.
func SeeSpellAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{21295, 21297, 21299}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			selfBuffCreated(s, e.NPC)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			selfBuffTimer(s, e)
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if selfBuff, _ := n.SkillByType(npc.SkillTypeSelfBuff); s.AbnormalLevel(n, selfBuff) <= 0 {
				castByType(n, n, npc.SkillTypeSelfBuff)
			}
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil {
					if s.Rnd(100) < 33 && script.Same(mostHated, attacker) {
						castPhysicalSpecial(n, attacker)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Skill.ID != 0 {
				base.ClanAttacked(s, e)
			}
		},
	})}
}
