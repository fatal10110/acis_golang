package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const castSleepMagicDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastSleepMagic"

// CastSleepMagic is WarriorCastSleepMagic: a Warrior that, once hit, every
// 5 s while it attacks or casts may put the one it hates most to sleep,
// unless that one hit it since; a player's or summon's hit, past the
// first, by one it does not hate most may draw the sleep on the attacker
// while it holds over a tenth of its HP.
func CastSleepMagic() script.Script {
	return script.Script{Dir: castSleepMagicDir, Behavior: true, NPCs: []int32{20284, 20283, 20276, 21023, 20078}, Hooks: castSleepMagicHooks()}
}

// castSleepMagicHooks are WarriorCastSleepMagic's hooks, which its child
// derives theirs from.
func castSleepMagicHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI2) == 0 {
					if mostHated := n.MostHated(); mostHated != nil && s.Rnd(100) < 50 {
						if sleepMagic, ok := n.SkillByType(npc.SkillTypeSleepMagic); s.AbnormalLevel(mostHated, sleepMagic) <= 0 && ok {
							n.AddCastDesire(mostHated, sleepMagic, 1000000)
						}
					}
				}
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			if n.Int(script.SlotIntAI1) == 1 {
				if script.Same(mostHated, attacker) {
					n.SetInt(script.SlotIntAI2, 1)
				}
			} else {
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 1)
			}
			if attacker.Playable() {
				if n.Int(script.SlotIntAI0) == 0 {
					n.SetInt(script.SlotIntAI0, 1)
				} else if n.Int(script.SlotIntAI0) == 1 && !script.Same(mostHated, attacker) && s.Rnd(100) < 30 && n.HPRatio() > 0.1 {
					addCastByType(n, attacker, npc.SkillTypeSleepMagic, 1000000)
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorCastSleepMagic")
}

// CastSleepMagicAggressive is WarriorCastSleepMagicAggressive: a
// WarriorCastSleepMagic that attacks the players and summons it sees.
func CastSleepMagicAggressive() script.Script {
	base := castSleepMagicHooks()
	return script.Script{Dir: castSleepMagicDir, Behavior: true, NPCs: []int32{20673, 20048, 20353}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}
