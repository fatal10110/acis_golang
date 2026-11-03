package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// msgTrapFailed is the system message telling a player the capture
// failed.
const msgTrapFailed = 2002

// The capture skills a player casts on a tyrannosaurus; the one of id
// captureSkillA+i matches the social animation i the beast last played.
const (
	captureSkillA = 5106
	captureSkillB = 5107
	captureSkillC = 5108
)

// TyrannoPrimeval is TyrannoPrimeval: the tyrannosaurus of the Primeval
// Isle, a Warrior that buffs itself as it weakens, raises its skill
// chances by a multiplier while buffed, and can be captured: a capture
// skill matching the social animation it last played debuffs it and arms
// the capture cycle, a wrong one makes it cancel that skill. Once event
// 11049 told it to report, it broadcasts event 11051 when it has been
// left alone for a while and shouts as it dies. With CollectGhost set it
// leaves the world once out of combat after CollectGhostDespawnTime
// minutes.
//
// Its memory: IntAI3 is the multiplier of its skill chances, IntAI4 the
// tick of its last fight or sighting, IntQuest0 the capture stage,
// IntQuest1 the social animation it last played, IntQuest2 is set once it
// was told to report; Creature0 is the last capturer it saw fail,
// Creature1 the target it dropped to buff itself.
func TyrannoPrimeval() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/TyrannoPrimeval", Behavior: true, NPCs: []int32{22215, 22216}, Hooks: tyrannoPrimevalHooks()}
}

// tyrannoPrimevalHooks are TyrannoPrimeval's hooks, which its child
// derives its own from.
func tyrannoPrimevalHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI3, 1)
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			n.SetInt(npc.IntQuest0, 0)
			n.SetInt(npc.IntQuest1, 0)
			n.SetInt(npc.IntQuest2, 0)
			if n.IntParam("CollectGhost") == 1 {
				s.StartTimer("9009", n, nil, ghostDespawn(n))
			}
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if !n.InCombat() {
				if !n.Dead() {
					n.SetInt(npc.IntQuest0, 0)
				}
				n.SetInt(npc.IntQuest1, 0)
				if n.Int(script.SlotIntAI3) > 1 {
					addCastByType(n, n, npc.SkillTypeSelfBuff1, 1000000)
					n.SetInt(script.SlotIntAI3, 1)
				}
				if n.IntParam("mobile_type") == 0 {
					n.AddMoveToDesire(n.SpawnLocation(), 30)
				}
			}
			if n.Int(npc.IntQuest2) == 1 && s.ElapsedTicks(n.Int(script.SlotIntAI4)) > 600 {
				s.BroadcastScriptEvent(n, 11051, 0, 8000)
			}
			base.NoDesire(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "9001"):
				if n.Int(npc.IntQuest0) > 0 && n.InCombat() && int32(s.Rnd(100)) < n.IntParamOr("ProbTimer", 100) {
					breakCapture(s, n)
				}
			case strings.EqualFold(e.Name, "9002"):
				if s.ElapsedTicks(n.Int(script.SlotIntAI4)) > 60 && n.Intention() != script.IntentionAttack {
					n.HateList().Refresh()
					s.BroadcastScriptEvent(n, 11051, 0, 8000)
				}
				s.StartTimer("9002", n, nil, 300000*time.Millisecond)
			case strings.EqualFold(e.Name, "9009"):
				if !n.InCombat() {
					n.DeleteMe()
				} else if n.IntParam("CollectGhost") == 1 {
					s.StartTimer("9009", n, nil, ghostDespawn(n))
				}
			}
			return base.Timer(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.InCombat() {
				// The rates are percents, the ratio a share of 1: every
				// ratio passes the first rate above 1.
				hpRatio := n.HPRatio()
				if hpRatio < float64(n.IntParamOr("HpChkRate5", 30)) {
					if isSkillOfType(n, e.Skill, npc.SkillTypeSelfBuff1) {
						n.SetInt(script.SlotIntAI3, 1)
						resumeDroppedTarget(n)
					} else if isSkillOfType(n, e.Skill, npc.SkillTypeSelfBuff2) {
						n.SetInt(script.SlotIntAI3, n.IntParamOr("ProbMultiplier2", 5))
						resumeDroppedTarget(n)
					}
				} else if hpRatio < float64(n.IntParamOr("HpChkRate3", 60)) {
					if isSkillOfType(n, e.Skill, npc.SkillTypeSelfBuff1) {
						n.SetInt(script.SlotIntAI3, n.IntParamOr("ProbMultiplier2", 3))
						resumeDroppedTarget(n)
					}
				}
			}
			if e.Success {
				if c0 := n.Slot(script.SlotCreature0); c0 != nil && (isSkillOfType(n, e.Skill, npc.SkillTypeCaptureCancelA) || isSkillOfType(n, e.Skill, npc.SkillTypeCaptureCancelB) || isSkillOfType(n, e.Skill, npc.SkillTypeCaptureCancelC)) {
					p := script.ActingPlayer(c0)
					if p == nil {
						// No player to tell: the hook stops here.
						return
					}
					p.SystemMessage(msgTrapFailed)
				}
				if e.Creature != nil && (isSkillOfType(n, e.Skill, npc.SkillTypeLongRangeDDMagic1) || isSkillOfType(n, e.Skill, npc.SkillTypePhysicalSpecial1) || isSkillOfType(n, e.Skill, npc.SkillTypePhysicalSpecial2) || isSkillOfType(n, e.Skill, npc.SkillTypePhysicalSpecial3)) && s.Rnd(100) < 3 {
					breakCapture(s, n)
				}
			}
		},
		OnAbnormalStatusChanged: func(s *script.Script, e script.AbnormalStatusChanged) {
			n, ref := e.NPC, e.Skill
			if ref.ID == captureSkillA || ref.ID == captureSkillB || ref.ID == captureSkillC {
				if ref.Level == 0 && n.Int(npc.IntQuest0) > 0 {
					addCastByType(n, n, npc.SkillTypeCaptureCancelAll, 10000000)
					if !n.Dead() {
						n.SetInt(npc.IntQuest0, 0)
					}
				} else if ref.Level > 0 {
					i0 := int32(ref.ID - captureSkillA)
					if i0 != n.Int(npc.IntQuest1) {
						switch i0 {
						case 0:
							addCastByType(n, n, npc.SkillTypeCaptureCancelA, 10000000)
						case 1:
							addCastByType(n, n, npc.SkillTypeCaptureCancelB, 10000000)
						case 2:
							addCastByType(n, n, npc.SkillTypeCaptureCancelC, 10000000)
						}
						n.SetSlot(script.SlotCreature0, e.Caster)
					} else if ref.Level == 1 && n.Int(npc.IntQuest0) == 0 {
						addCastByType(n, n, npc.SkillTypeDebuff1, 10000000)
						if !n.Dead() {
							n.SetInt(npc.IntQuest0, 1)
						}
						s.StartTimer("9001", n, nil, captureCycle(s, n))
					} else if ref.Level >= 2 && n.Int(npc.IntQuest0) >= 1 {
						addCastByType(n, n, npc.SkillTypeDebuff2, 10000000)
						if !n.Dead() {
							n.SetInt(npc.IntQuest0, 1)
						}
						s.StartTimer("9001", n, nil, captureCycle(s, n))
					}
				}
			}
			base.AbnormalStatusChanged(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			hpRatio := n.HPRatio()
			topDesireTarget := n.TopDesireTarget()
			// The rates are percents, the ratio a share of 1: every ratio
			// passes the first rate above 1.
			if hpRatio <= float64(n.IntParamOr("HpChkRate5", 30)) {
				if n.Int(script.SlotIntAI3) == n.IntParamOr("ProbMultiplier1", 3) {
					dropTargetToBuff(n, topDesireTarget, npc.SkillTypeSelfBuff1)
				} else if n.Int(script.SlotIntAI3) == 1 {
					dropTargetToBuff(n, topDesireTarget, npc.SkillTypeSelfBuff2)
				}
			} else if hpRatio <= float64(n.IntParamOr("HpChkRate5", 60)) {
				if n.Int(script.SlotIntAI3) == 1 {
					dropTargetToBuff(n, topDesireTarget, npc.SkillTypeSelfBuff1)
				}
			}
			multiplier := n.Int(script.SlotIntAI3)
			if n.Distance3D(attacker) > float64(n.IntParamOr("LongRangeSkillDist", 0)) {
				if int32(s.Rnd(100)) <= n.IntParamOr("ProbLongRangeDDMagic1", 0)*multiplier {
					addCastByType(n, attacker, npc.SkillTypeLongRangeDDMagic1, 1000000)
				}
			} else if topDesireTarget != nil && !script.Same(topDesireTarget, n) {
				if int32(s.Rnd(100)) <= n.IntParamOr("ProbLongRangeDDMagic1", 0)*multiplier {
					addCastByType(n, topDesireTarget, npc.SkillTypeLongRangeDDMagic1, 1000000)
				}
				if int32(s.Rnd(100)) <= n.IntParamOr("ProbPhysicalSpecial1", 0)*multiplier {
					addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 1000000)
				}
				if int32(s.Rnd(100)) <= n.IntParamOr("ProbPhysicalSpecial2", 0)*multiplier {
					addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial2, 1000000)
				}
				if int32(s.Rnd(100)) <= n.IntParamOr("ProbPhysicalSpecial3", 0)*multiplier {
					addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial3, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 11049:
				n.SetInt(npc.IntQuest2, 1)
				s.StartTimer("9002", n, nil, 600000*time.Millisecond)
			case eventShoutTarget:
				if c0 := s.Object(e.Arg1); isPlayer(c0) && n.IntParamOr("BroadCastReception", 0) == 1 {
					n.RemoveAllAttackDesire()
					n.AddAttackDesire(c0, 100)
				}
			}
			base.ScriptEvent(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			if isPlayer(c) && !n.InCombat() && n.IntParamOr("ag_type", 0) == 1 {
				addCastByType(n, c, npc.SkillTypeLongRangeDDMagic1, 1000000)
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if n.Int(npc.IntQuest2) == 1 {
				n.Shout(script.NpcString(1800011))
				s.BroadcastScriptEventEx(n, 11044, 0, 0, 5000)
			}
			base.MyDying(s, e)
		},
	}).Named("TyrannoPrimeval")
}

// ghostDespawn is how long a collected ghost stays out of combat before it
// leaves the world.
func ghostDespawn(n *script.NPC) time.Duration {
	return time.Duration(int64(n.IntParamOr("CollectGhostDespawnTime", 30))*60*1000) * time.Millisecond
}

// captureCycle draws the delay of the capture timer.
func captureCycle(s *script.Script, n *script.NPC) time.Duration {
	seconds := n.IntParamOr("CaptureCycle", 10) + int32(s.Rnd(int(n.IntParamOr("CaptureCycleRand", 4))))
	return time.Duration(seconds*1000) * time.Millisecond
}

// breakCapture makes n want to cancel every capture on it, resets its
// capture stage and plays one of its three social animations, drawn.
func breakCapture(s *script.Script, n *script.NPC) {
	addCastByType(n, n, npc.SkillTypeCaptureCancelAll, 10000000)
	if !n.Dead() {
		n.SetInt(npc.IntQuest0, 0)
	}
	social := int32(s.Rnd(3))
	n.SetInt(npc.IntQuest1, social)
	var id, frame int32
	switch social {
	case 0:
		id, frame = n.IntParamOr("Social1", 0), n.IntParamOr("Social1_Frame", 10)
	case 1:
		id, frame = n.IntParamOr("Social2", 1), n.IntParamOr("Social2_Frame", 10)
	case 2:
		id, frame = n.IntParamOr("Social3", 2), n.IntParamOr("Social3_Frame", 10)
	}
	if frame > 0 {
		n.AddSocialDesire(int(id), int(frame), 10000000)
	}
}

// dropTargetToBuff makes n remember top, when it has one, drop its attack
// desires and want its skill of type t on itself.
func dropTargetToBuff(n *script.NPC, top script.Creature, t npc.SkillType) {
	if top != nil {
		n.SetSlot(script.SlotCreature1, top)
	}
	n.RemoveAllAttackDesire()
	addCastByType(n, n, t, 1000000)
}

// resumeDroppedTarget makes n attack again the playable it dropped to buff
// itself, and forgets it.
func resumeDroppedTarget(n *script.NPC) {
	if c1 := n.Slot(script.SlotCreature1); c1 != nil {
		if c1.Playable() {
			n.AddAttackDesire(c1, 100)
		}
		n.SetSlot(script.SlotCreature1, nil)
	}
}
