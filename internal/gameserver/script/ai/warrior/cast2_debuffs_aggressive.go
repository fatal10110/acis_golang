package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Cast2DebuffsAggressive is WarriorCast2DebuffsAggressive: an aggressive
// Warrior that may lay its two debuffs on a player or summon it sees while
// it wanders, on its most hated attacker and on a clan member's attacker,
// and once hit, every 5 s while it fights, its first debuff on the creature
// it hates most.
func Cast2DebuffsAggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21379}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
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
						debuff1, ok := n.SkillByType(npc.SkillTypeDebuff1)
						if s.AbnormalLevel(mostHated, debuff1) <= 0 && ok {
							n.AddCastDesire(mostHated, debuff1, 1000000)
						}
					}
				}
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.Intention() == script.IntentionWander {
				maybeDebuff(s, n, c, npc.SkillTypeDebuff1)
				maybeDebuff(s, n, c, npc.SkillTypeDebuff2)
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
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
			if attacker.Playable() && script.Same(mostHated, attacker) {
				maybeDebuff(s, n, attacker, npc.SkillTypeDebuff1)
				maybeDebuff(s, n, attacker, npc.SkillTypeDebuff2)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				maybeDebuff(s, called, attacker, npc.SkillTypeDebuff1)
				maybeDebuff(s, called, attacker, npc.SkillTypeDebuff2)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// maybeDebuff makes n want to cast its template's skill of type t on c,
// with weight 1000000, on a draw under 33 when c holds no effect of the
// skill's stack. The draw is made even when the template lists no such
// skill, which then casts nothing.
func maybeDebuff(s *script.Script, n *script.NPC, c script.Creature, t npc.SkillType) {
	debuff, ok := n.SkillByType(t)
	if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 && ok {
		n.AddCastDesire(c, debuff, 1000000)
	}
}
