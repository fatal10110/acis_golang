package warrior

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderCastSplashAggressive is PartyLeaderCastSplashAggressive: a
// PartyLeaderWarriorAggressive that may cast its self-centred DD magic when
// the creature it hates most hits it, or a clan member, from closer than
// 200.
func PartyLeaderCastSplashAggressive() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21058, 20966, 20973, 21078, 20986}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) && n.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
					castByType(n, n, npc.SkillTypeSelfRangeDDMagic)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if script.Same(called.MostHated(), attacker) && called.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
					castByType(called, called, npc.SkillTypeSelfRangeDDMagic)
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// PartyLeaderPabelgolemAggressive is PartyLeaderPabelgolemAggressive: a
// PartyLeaderWarriorAggressive that may strike an attacker with its
// physical special skill, or with its self-centred one when the attacker is
// in a party, and may lay its debuff on a private's attacker.
func PartyLeaderPabelgolemAggressive() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22060, 22074}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			player := script.ActingPlayer(attacker)
			inParty := player != nil && player.InParty()
			if !inParty && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial)
			} else if inParty && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypeSelfRangePhysicalSpecial)
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if s.Rnd(100) < 20 {
				castByType(e.Called, e.Target, npc.SkillTypeDebuff)
			}
			base.PartyAttacked(s, e)
		},
	})}
}

// PartyLeaderPatrolPhysicalSpecialStoneAggressive is
// PartyLeaderPatrolPhysicalSpecialStoneAggressive: a Ketra or Varka
// PartyLeaderWarriorAggressive that petrifies an attacker allied with its
// own faction, and a player who helps one, and ignores such a player it
// sees. Otherwise it may debuff and strike the creature it hates most, a
// clan member's attacker and a private's attacker, and buff itself or the
// private.
func PartyLeaderPatrolPhysicalSpecialStoneAggressive() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21343, 21369}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if defaultnpc.MaybeCastPetrify(n, attacker) {
					return
				}
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, attacker) {
					debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
						castByType(n, attacker, npc.SkillTypeDebuff)
					}
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypePhysicalSpecial)
					}
					if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 {
						castByType(n, n, npc.SkillTypeBuff)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if defaultnpc.MaybeCastPetrify(called, attacker) {
				return
			}
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypePhysicalSpecial)
				}
				debuff, _ := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
					castByType(called, attacker, npc.SkillTypeDebuff)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if called.LifeTime() > 7 && target.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, target, npc.SkillTypePhysicalSpecial)
				}
				debuff, _ := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 33 && s.AbnormalLevel(target, debuff) <= 0 {
					castByType(called, target, npc.SkillTypeDebuff)
				}
				if called.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && called.HPRatio() > 0.5 {
					castByType(called, e.Caller, npc.SkillTypeBuff)
				}
			}
			base.PartyAttacked(s, e)
		},
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
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

// PartyLeaderSlowTypeAggressive is PartyLeaderSlowTypeAggressive: a
// PartyLeaderWarriorAggressive that may slow a creature it sees standing
// away, a near attacker, and a clan member's attacker, and turns on a near
// attacker while it fights a far one. Once hit, every 5 s while it fights,
// it may slow the creature it hates most.
func PartyLeaderSlowTypeAggressive() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22092}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if c.Playable() && n.LifeTime() > 7 && n.InMyTerritory() {
				if n.Intention() != script.IntentionAttack && n.Distance2D(c) > 100 && s.Rnd(100) < 10 {
					castByType(n, c, npc.SkillTypeDDMagicSlow)
				}
			}
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			if mostHated != nil && !script.Same(attacker, mostHated) {
				if n.Intention() == script.IntentionAttack && n.Distance2D(mostHated) > 100 && n.Distance2D(attacker) < 100 && s.Rnd(100) < 80 {
					n.RemoveAllAttackDesire()
					if attacker.Playable() {
						hateRatio := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
						n.AddAttackDesire(attacker, hateRatio*30)
					}
				}
			}
			if n.Distance2D(attacker) < 200 && s.Rnd(100) < 10 {
				castByType(n, attacker, npc.SkillTypeDDMagicSlow)
			}
			if n.Int(script.SlotIntAI0) == 1 {
				if mostHated != nil && script.Same(mostHated, attacker) {
					n.SetInt(script.SlotIntAI1, 1)
				}
			} else {
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI0, 1)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI0, 0)
					n.SetInt(script.SlotIntAI1, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI1) == 0 {
					if mostHated := n.MostHated(); mostHated != nil && s.Rnd(100) < 50 {
						castByType(n, mostHated, npc.SkillTypeDDMagicSlow)
					}
				}
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI0, 0)
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if isPlayer(attacker) && called.LifeTime() > 7 {
				if called.Intention() != script.IntentionAttack && s.Rnd(100) < 10 {
					castByType(called, attacker, npc.SkillTypeDDMagicSlow)
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}
