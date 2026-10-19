package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// CorpseNecroLongAggressive is WarriorCorpseNecroLongAggressive: an
// aggressive Warrior that, while it attacks no one, may strike a player or
// summon it sees farther than 100 in its territory with its physical
// special, and, once it hates someone, may cast its second magic on a dead
// one it sees within 100. Its most hated attacker it strikes with its
// physical special from farther than 100 and, below 40% of its hit points,
// may hit with its first magic; for its clan it strikes an attacker
// farther than 100 with its physical special while it attacks no one.
func CorpseNecroLongAggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21574, 21567}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.Intention() != script.IntentionAttack {
				dist := n.Distance2D(c)
				if n.LifeTime() > 7 && dist > 100 && s.Rnd(100) < 33 && n.InMyTerritory() {
					addCastByType(n, c, npc.SkillTypePhysicalSpecial, 1000000)
				}
				if c.Dead() && dist < 100 && s.Rnd(100) < 33 {
					if n.MostHated() != nil {
						addCastByType(n, c, npc.SkillTypeDDMagic2, 1000000)
					}
				}
			}
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) {
					if n.Distance2D(attacker) > 100 {
						addCastByType(n, attacker, npc.SkillTypePhysicalSpecial, 1000000)
					}
					if s.Rnd(100) < 33 && n.HPRatio() < 0.4 {
						addCastByType(n, attacker, npc.SkillTypeDDMagic1, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && called.Distance2D(attacker) > 100 {
				addCastByType(called, attacker, npc.SkillTypePhysicalSpecial, 1000000)
			}
			base.ClanAttacked(s, e)
		},
	})}
}
