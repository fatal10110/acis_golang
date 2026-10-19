package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderWarriorAggressive is PartyLeaderWarriorAggressive: a
// PartyLeaderWarrior that attacks the players and summons it sees.
func PartyLeaderWarriorAggressive() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{20376, 20520, 20522}, Hooks: PartyLeaderWarriorAggressiveHooks()}
}

// PartyLeaderWarriorAggressiveHooks are PartyLeaderWarriorAggressive's
// hooks, which its children derive theirs from.
func PartyLeaderWarriorAggressiveHooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	}).Named("PartyLeaderWarriorAggressive")
}

// FreyaGardener is FreyaGardener: a PartyLeaderWarriorAggressive that lays
// its debuff on the first three distinct players or summons that hit it
// while free of it, and otherwise may hold those around it. It stops
// hating whoever its debuff lands on, and as it dies it tells the Garden
// Sculpture's maker.
func FreyaGardener() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22100}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, 0)
			n.SetSlot(script.SlotCreature0, nil)
			n.SetSlot(script.SlotCreature1, nil)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
				if n.Int(script.SlotIntAI1) < 3 && s.AbnormalLevel(attacker, debuff) == -1 {
					switch n.Int(script.SlotIntAI1) {
					case 0:
						n.SetSlot(script.SlotCreature0, attacker)
						addCastByType(n, n.Slot(script.SlotCreature0), npc.SkillTypeDebuff, 1000000)
					case 1:
						if !script.Same(n.Slot(script.SlotCreature0), attacker) {
							n.SetSlot(script.SlotCreature1, attacker)
							addCastByType(n, n.Slot(script.SlotCreature1), npc.SkillTypeDebuff, 1000000)
						}
					case 2:
						if !script.Same(n.Slot(script.SlotCreature0), attacker) && !script.Same(n.Slot(script.SlotCreature1), attacker) {
							addCastByType(n, attacker, npc.SkillTypeDebuff, 1000000)
						}
					}
					n.SetInt(script.SlotIntAI1, n.Int(script.SlotIntAI1)+1)
				} else if s.Rnd(100) < 20 {
					addCastByType(n, n, npc.SkillTypeRangeHoldA, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			if debuff, ok := e.NPC.SkillByType(npc.SkillTypeDebuff); ok && e.Skill == debuff {
				e.NPC.StopHate(e.Creature)
			}
			base.UseSkillFinished(s, e)
		},
		OnMyDying: func(s *script.Script, _ script.MyDying) {
			s.Spawner().SendMakerEvent("schuttgart13_npc2314_2m1", "10005", 0, 0)
		},
	})}
}

// PartyLeaderCannibaliAggressiva is PartyLeaderCannibaliAggressiva: a
// PartyLeaderWarriorAggressive whose attacked hook calls no parent. It
// hates by the damage it takes, buffs itself once under half its HP, and
// strikes and pulls from afar on its chances. Under 70% of its HP after a
// party member fell under a third of its own, it turns into its
// transformed self (22114); under a third of its HP it sends its privates
// away.
func PartyLeaderCannibaliAggressiva() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22113}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetSlot(script.SlotCreature0, n)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetSlot(script.SlotCreature0, attacker)
			if attacker.Playable() {
				damage := e.Damage
				if damage == 0 {
					damage = 1
				}
				n.AddAttackDesire(attacker, float64(damage)/float64(n.Level()+7)*1000)
			}
			if n.HPRatio() < 0.5 && n.Int(script.SlotIntAI1) == 0 {
				addCastByType(n, n, npc.SkillTypeBuff, 1000000)
				n.SetInt(script.SlotIntAI1, 1)
			}
			if s.Rnd(100) < 33 {
				addCastByType(n, attacker, npc.SkillTypePhysicalSpecial1, 1000000)
			}
			dist := n.Distance2D(attacker)
			if dist > 300 && dist < 1000 && s.Rnd(100) < 10 {
				addCastByType(n, attacker, npc.SkillTypeSummonMagic, 1000000)
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called := e.Caller, e.Called
			if !script.Same(called, caller) {
				if caller.HPRatio() < 0.33 {
					called.SetInt(script.SlotIntAI3, 1)
					called.SetSlot(script.SlotCreature1, caller)
				}
			} else if called.Int(script.SlotIntAI0) == 0 {
				hpRatio := called.HPRatio()
				if hpRatio < 0.7 && called.Int(script.SlotIntAI3) == 1 {
					called.SetInt(script.SlotIntAI0, 1)
					addCastByType(called, called.Slot(script.SlotCreature1), npc.SkillTypeEffectSkill, 1000000)
				}
				if hpRatio < 0.33 && !caller.Dead() {
					called.SetInt(script.SlotIntAI0, 1)
					addCastByType(called, called, npc.SkillTypeEffectSkill2, 1000000)
				}
			}
			base.PartyAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if c0 := n.Slot(script.SlotCreature0); c0 != nil {
				if summon, ok := n.SkillByType(npc.SkillTypeSummonMagic); ok && e.Skill == summon && n.Distance2D(c0) < 1000 && s.Rnd(100) < 10 {
					c0.InstantTeleportTo(script.LocOf(n))
					n.AddAttackDesire(c0, 100000)
				}
				if effect, ok := n.SkillByType(npc.SkillTypeEffectSkill); ok && e.Skill == effect {
					at := script.LocOf(n)
					s.Spawner().CreateOnePrivateEx(n, 22114, script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{P1: 1000, P2: c0.ObjectID()})
					n.DeleteMe()
					s.BroadcastScriptEvent(n, 10028, n.ObjectID(), 500)
				}
			}
			if effect2, ok := n.SkillByType(npc.SkillTypeEffectSkill2); ok && e.Skill == effect2 {
				s.BroadcastScriptEvent(n, 10028, n.ObjectID(), 500)
			}
		},
	})}
}

// PartyLeaderCastCurseAggressive is PartyLeaderCastCurseAggressive: a
// PartyLeaderWarriorAggressive that may lay its debuff on its most hated
// attacker and on a clan member's attacker, and once hit, every 5 s while
// it fights, on the creature it hates most, each only on a creature free
// of the debuff.
func PartyLeaderCastCurseAggressive() script.Script {
	base := PartyLeaderWarriorAggressiveHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{20773, 21081}, Hooks: base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if !fighting(n.Intention()) {
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(script.SlotIntAI2, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI2) == 0 {
					if debuff, ok := n.SkillByType(npc.SkillTypeDebuff); ok && s.Rnd(100) < 50 {
						if mostHated := n.MostHated(); mostHated != nil && s.AbnormalLevel(mostHated, debuff) <= 0 {
							n.AddCastDesire(mostHated, debuff, 1000000)
						}
					}
				}
				s.StartTimer("2001", n, e.Player, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			armFightTimer(s, n, attacker, mostHated)
			if attacker.Playable() && script.Same(mostHated, attacker) {
				if debuff, ok := n.SkillByType(npc.SkillTypeDebuff); ok && s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
					n.AddCastDesire(attacker, debuff, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if debuff, ok := called.SkillByType(npc.SkillTypeDebuff); ok && s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
					called.AddCastDesire(attacker, debuff, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}
