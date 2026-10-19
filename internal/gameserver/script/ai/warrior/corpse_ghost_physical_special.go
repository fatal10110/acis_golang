package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// CorpseGhostPhysicalSpecial is WarriorCorpseGhostPhysicalSpecial: a
// Warrior that casts its self buff at its creation and, every 10 seconds
// while it fights no one, again when it lost it, and that strikes with its
// physical special skill the player it hates most when hit by that one,
// or the attacker of its clan while it does not fight.
func CorpseGhostPhysicalSpecial() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21555}, Hooks: corpseGhostPhysicalSpecialHooks()}
}

// corpseGhostPhysicalSpecialHooks are WarriorCorpseGhostPhysicalSpecial's
// hooks, which its child derives its own from.
func corpseGhostPhysicalSpecialHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("2001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			castByType(n, n, npc.SkillTypeSelfBuff)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypePhysicalSpecial)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypePhysicalSpecial)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if n.Intention() != script.IntentionAttack {
					selfBuff, _ := n.SkillByType(npc.SkillTypeSelfBuff)
					if s.AbnormalLevel(n, selfBuff) <= 0 {
						castByType(n, n, npc.SkillTypeSelfBuff)
					}
				}
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorCorpseGhostPhysicalSpecial")
}

// CorpseGhostPhysicalSpecialAggressive is
// WarriorCorpseGhostPhysicalSpecialAggressive: a
// WarriorCorpseGhostPhysicalSpecial that attacks the players and summons
// it sees and, settled in its territory and fighting, may strike the one
// it hates most with its physical special skill on seeing that one.
func CorpseGhostPhysicalSpecialAggressive() script.Script {
	base := corpseGhostPhysicalSpecialHooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.Intention() == script.IntentionAttack {
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, c) && s.Rnd(100) < 33 {
					castByType(n, mostHated, npc.SkillTypePhysicalSpecial)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21571}, Hooks: hooks}
}
