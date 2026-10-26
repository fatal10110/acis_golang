package frintezza

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PortraitSpirit (29050), the spirit an EvilateA raises. Appearing it
// wanders 200 s at weight 5 and checks its master every 10 s: with no live
// master it leaves the world unless it attacks or casts at a target within
// 5000. A playable that hits it, or that it sees or a clan member calls it
// on after 7 AI cycles, draws its physical special skill and an attack; a
// hit also makes it cast its hold 5 s later. After each cast it presses its
// player target with the vampiric touch, three times at most, while the
// target is held, its physical special skill otherwise. Script event arg1
// 20000 makes it cast its bomb, decaying once the bomb lands; 40000 makes
// it leave the world; 50000 calms it for good.
func PortraitSpirit() script.Script {
	spiritBomb := skill.Ref{ID: 5011, Level: 1}
	spiritHold := skill.Ref{ID: 5012, Level: 1}
	spiritVampiric := skill.Ref{ID: 5013, Level: 1}
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29050}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI4, 0)
			n.SetInt(npc.IntQuest1, 0)
			n.SetInt(npc.IntQuest2, 0)
			n.AddWanderDesire(200, 5)
			s.StartTimer("5000", n, nil, 10*1000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 && e.Attacker.Playable() {
				castTyped(n, e.Attacker, npc.SkillTypePhysicalSpecial)
				n.AddAttackDesire(e.Attacker, 50)
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			n := e.Called
			if n.Int(script.SlotIntAI4) == 0 && n.LifeTime() > 7 && e.Attacker.Playable() {
				castTyped(n, e.Attacker, npc.SkillTypePhysicalSpecial)
				n.AddAttackDesire(e.Attacker, 50)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if e.Skill == spiritBomb && e.Success {
				s.Spawner().DecayMe(n)
				return
			}
			if n.Int(script.SlotIntAI4) != 0 {
				return
			}
			top, ok := n.TopDesireTarget().(*script.Player)
			if !ok {
				return
			}
			if s.AbnormalLevel(top, spiritHold) > 0 {
				if n.Int(npc.IntQuest2) < 3 {
					n.SetInt(npc.IntQuest2, n.Int(npc.IntQuest2)+1)
					n.AddCastDesire(top, spiritVampiric, 1000000)
				}
			} else {
				castTyped(n, top, npc.SkillTypePhysicalSpecial)
			}
			n.AddAttackDesire(top, 50)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 {
				top := n.TopDesireTarget()
				switch {
				case strings.EqualFold(e.Name, "2001"):
					if top != nil && s.ElapsedTicks(n.Int(npc.IntQuest1)) > 5 {
						n.AddCastDesire(top, spiritHold, 1000000)
					}
				case strings.EqualFold(e.Name, "2002"):
					if top != nil {
						if s.AbnormalLevel(top, spiritHold) > 0 {
							if n.Int(npc.IntQuest2) < 3 {
								n.SetInt(npc.IntQuest2, n.Int(npc.IntQuest2)+1)
								n.AddCastDesire(top, spiritVampiric, 1000000)
							}
							s.StartTimer("2002", n, nil, 5000*time.Millisecond)
						} else {
							castTyped(n, top, npc.SkillTypePhysicalSpecial)
						}
						n.AddAttackDesire(top, 50)
					}
				case strings.EqualFold(e.Name, "5000"):
					if !n.HasMaster() || n.Master().Dead() {
						switch {
						case n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast:
							n.DeleteMe()
						case top != nil && n.Distance2D(top) > 5000:
							n.DeleteMe()
						default:
							s.StartTimer("5000", n, e.Player, 10000*time.Millisecond)
						}
					} else {
						s.StartTimer("5000", n, e.Player, 10000*time.Millisecond)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n := e.NPC
			if e.Creature.Playable() && n.Int(script.SlotIntAI4) == 0 && n.LifeTime() > 7 {
				castTyped(n, e.Creature, npc.SkillTypePhysicalSpecial)
				n.AddAttackDesire(e.Creature, 50)
			}
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.Arg1 {
			case 20000:
				n.RemoveAllDesire()
				n.AddCastDesire(n, spiritBomb, 1000000)
			case 40000:
				n.RemoveAllDesire()
				n.DeleteMe()
			case 50000:
				n.SetInt(script.SlotIntAI4, 1)
				n.RemoveAllDesire()
			}
		},
	})}
}

// PortraitSpiritWinged (29051), the spirit an EvilateB raises: a
// PortraitSpirit fighting with its DD magic.
func PortraitSpiritWinged() script.Script {
	spiritBomb := skill.Ref{ID: 5011, Level: 1}
	spiritHold := skill.Ref{ID: 5012, Level: 1}
	spiritVampiric := skill.Ref{ID: 5013, Level: 1}
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29051}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI4, 0)
			n.SetInt(npc.IntQuest1, 0)
			n.SetInt(npc.IntQuest2, 0)
			n.AddWanderDesire(200, 5)
			s.StartTimer("5000", n, nil, 10*1000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 && e.Attacker.Playable() {
				castTyped(n, e.Attacker, npc.SkillTypeDDMagic)
				n.AddAttackDesire(e.Attacker, 50)
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			n := e.Called
			if n.Int(script.SlotIntAI4) == 0 && n.LifeTime() > 7 && e.Attacker.Playable() {
				castTyped(n, e.Attacker, npc.SkillTypeDDMagic)
				n.AddAttackDesire(e.Attacker, 50)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if e.Skill == spiritBomb && e.Success {
				s.Spawner().DecayMe(n)
				return
			}
			if n.Int(script.SlotIntAI4) != 0 {
				return
			}
			top, ok := n.TopDesireTarget().(*script.Player)
			if !ok {
				return
			}
			if s.AbnormalLevel(top, spiritHold) > 0 {
				if n.Int(npc.IntQuest2) < 3 {
					n.SetInt(npc.IntQuest2, n.Int(npc.IntQuest2)+1)
					n.AddCastDesire(top, spiritVampiric, 1000000)
				}
			} else {
				castTyped(n, top, npc.SkillTypeDDMagic)
			}
			n.AddAttackDesire(top, 50)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 {
				top := n.TopDesireTarget()
				switch {
				case strings.EqualFold(e.Name, "2001"):
					if top != nil && s.ElapsedTicks(n.Int(npc.IntQuest1)) > 5 {
						n.AddCastDesire(top, spiritHold, 1000000)
					}
				case strings.EqualFold(e.Name, "2002"):
					if top != nil {
						if s.AbnormalLevel(top, spiritHold) > 0 {
							if n.Int(npc.IntQuest2) < 3 {
								n.SetInt(npc.IntQuest2, n.Int(npc.IntQuest2)+1)
								n.AddCastDesire(top, spiritVampiric, 1000000)
							}
							s.StartTimer("2002", n, nil, 5000*time.Millisecond)
						} else {
							castTyped(n, top, npc.SkillTypeDDMagic)
						}
						n.AddAttackDesire(top, 50)
					}
				case strings.EqualFold(e.Name, "5000"):
					if !n.HasMaster() || n.Master().Dead() {
						switch {
						case n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast:
							n.DeleteMe()
						case top != nil && n.Distance2D(top) > 5000:
							n.DeleteMe()
						default:
							s.StartTimer("5000", n, e.Player, 10000*time.Millisecond)
						}
					} else {
						s.StartTimer("5000", n, e.Player, 10000*time.Millisecond)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n := e.NPC
			if e.Creature.Playable() && n.Int(script.SlotIntAI4) == 0 && n.LifeTime() > 7 {
				castTyped(n, e.Creature, npc.SkillTypeDDMagic)
				n.AddAttackDesire(e.Creature, 50)
			}
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.Arg1 {
			case 20000:
				n.RemoveAllDesire()
				n.AddCastDesire(n, spiritBomb, 1000000)
			case 40000:
				n.RemoveAllDesire()
				n.DeleteMe()
			case 50000:
				n.SetInt(script.SlotIntAI4, 1)
				n.RemoveAllDesire()
			}
		},
	})}
}

// castTyped makes n want to cast the skill its template lists under t at
// target; a template listing none casts nothing.
func castTyped(n *script.NPC, target script.Creature, t npc.SkillType) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, 1000000)
	}
}
