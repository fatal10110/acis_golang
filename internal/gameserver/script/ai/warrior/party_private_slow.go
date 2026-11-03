package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyPrivatePriestBuffer is PartyPrivatePriestBuffer: a
// PartyPrivateWarrior that looks around every 20 s and, seeing its master
// or a private of the same master while it is not attacking, wants to cast
// each of its six buffs the creature holds none of.
func PartyPrivatePriestBuffer() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22131}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("5001", e.NPC, nil, 20000*time.Millisecond, 20000*time.Millisecond)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			master := n.Master()
			if master == nil {
				base.SeeCreature(s, e)
				return
			}
			if fellow, ok := c.(*script.NPC); (ok && fellow.HasMaster() && script.Same(fellow.Master(), master)) || script.Same(c, master) {
				if n.Intention() != script.IntentionAttack {
					for _, t := range []npc.SkillType{
						npc.SkillTypeBuff1, npc.SkillTypeBuff2, npc.SkillTypeBuff3,
						npc.SkillTypeBuff4, npc.SkillTypeBuff5, npc.SkillTypeBuff6,
					} {
						if buff, known := n.SkillByType(t); s.AbnormalLevel(c, buff) <= 0 && known {
							n.AddCastDesire(c, buff, 1000000)
						}
					}
				}
			}
			base.SeeCreature(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "5001") {
				e.NPC.LookNeighbor()
			}
			return base.Timer(s, script.Timer{Name: e.Name, NPC: e.NPC})
		},
	})}
}

// PartyPrivateSlowTypeMagic is PartyPrivateSlowTypeMagic: a
// PartyPrivateWarrior that may slow an attacker standing more than 200
// away, and a clan member's attacker while it is not attacking, and every
// 20 s of a fight may slow its most hated creature.
func PartyPrivateSlowTypeMagic() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20964}, Hooks: PartyPrivateSlowTypeMagicHooks()}
}

// PartyPrivateSlowTypeMagicHooks are PartyPrivateSlowTypeMagic's hooks,
// which its child derives its own from.
func PartyPrivateSlowTypeMagicHooks() script.Hooks {
	base := PartyPrivateHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if i := n.Intention(); i != script.IntentionAttack && i != script.IntentionCast {
					n.SetInt(script.SlotIntAI0, 0)
					n.SetInt(script.SlotIntAI1, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI1) == 0 {
					if mostHated := n.MostHated(); mostHated != nil && s.Rnd(100) < 50 {
						castByType(n, mostHated, npc.SkillTypeDDMagicSlow)
					}
				}
				s.StartTimer("2001", n, nil, 20000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 0)
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Distance2D(attacker) > 200 && s.Rnd(100) < 70 {
				castByType(n, attacker, npc.SkillTypeDDMagicSlow)
			}
			if n.Int(script.SlotIntAI0) == 1 {
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, attacker) {
					n.SetInt(script.SlotIntAI1, 1)
				}
			} else {
				s.StartTimer("2001", n, nil, 20000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 0)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && s.Rnd(100) < 10 {
				castByType(called, attacker, npc.SkillTypeDDMagicSlow)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyPrivateSlowTypeMagic")
}

// PartyPrivateSlowType2 is PartyPrivateSlowType2: a
// PartyPrivateSlowTypeMagic that draws once more to slow an attacker
// standing more than 200 away.
func PartyPrivateSlowType2() script.Script {
	base := PartyPrivateSlowTypeMagicHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22091}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Distance2D(attacker) > 200 && s.Rnd(100) < 70 {
				castByType(n, attacker, npc.SkillTypeDDMagicSlow)
			}
			base.Attacked(s, e)
		},
	})}
}
