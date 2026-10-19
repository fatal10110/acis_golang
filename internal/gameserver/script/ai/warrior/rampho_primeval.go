package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// RamphoPrimeval is RamphoPrimeval: the Pterosaur, a Warrior that buffs
// itself once low on hit points, strikes harder below a hit point share,
// walks home when it has nothing to do out of combat, and, once event 11049
// told it to report, broadcasts event 11051 when it has been left alone for
// more than 600 seconds.
//
// Its memory: IntAI2 is set once the self buff was wanted, IntAI3 is the
// multiplier of its skill chances, IntAI4 the tick of its last fight or
// sighting, and IntQuest0 is set once it was told to report.
func RamphoPrimeval() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22199}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(script.SlotIntAI3, 1)
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			n.SetInt(npc.IntQuest0, 0)
			base.Created(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if !n.InCombat() {
				n.SetInt(script.SlotIntAI2, 0)
				n.SetInt(script.SlotIntAI3, 1)
				if n.IntParam("mobile_type") == 0 {
					n.AddMoveToDesire(n.SpawnLocation(), 30)
				}
			}
			if n.Int(npc.IntQuest0) == 1 {
				if s.ElapsedTicks(n.Int(script.SlotIntAI4)) > 600 {
					s.BroadcastScriptEvent(n, 11051, 0, 8000)
				}
			}
			base.NoDesire(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			hpRatio := n.HPRatio()
			if hpRatio*100 <= float64(n.IntParamOr("HpChkRate4", 50)) {
				n.SetInt(script.SlotIntAI3, n.IntParamOr("ProbMultiplier1", 2))
			} else {
				n.SetInt(script.SlotIntAI3, 1)
			}
			if hpRatio*100 <= float64(n.IntParamOr("HpChkRate5", 30)) {
				if n.Int(script.SlotIntAI2) == 0 {
					addCastByType(n, n, npc.SkillTypeSelfBuff1, 10000000)
					n.SetInt(script.SlotIntAI2, 1)
				}
			}
			topDesireTarget := n.TopDesireTarget()
			if n.Distance2D(attacker) < float64(n.IntParamOr("LongRangeSkillDist", 100)) {
				if int32(s.Rnd(100)) <= n.IntParamOr("ProbLongRangeDDMagic1", 0)*n.Int(script.SlotIntAI3) {
					addCastByType(n, attacker, npc.SkillTypeLongRangeDDMagic1, 10000000)
				} else if topDesireTarget != nil {
					if int32(s.Rnd(100)) <= n.IntParamOr("ProbLongRangeDDMagic1", 0)*n.Int(script.SlotIntAI3) {
						addCastByType(n, topDesireTarget, npc.SkillTypeLongRangeDDMagic1, 10000000)
					}
					if int32(s.Rnd(100)) <= n.IntParamOr("ProbPhysicalSpecial1", 0)*n.Int(script.SlotIntAI3) {
						addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 10000000)
					}
					if int32(s.Rnd(100)) <= n.IntParamOr("ProbPhysicalSpecial2", 0)*n.Int(script.SlotIntAI3) {
						addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial2, 10000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 11049:
				n.SetInt(npc.IntQuest0, 1)
			case eventShoutTarget:
				c0 := s.Object(e.Arg1)
				if isPlayer(c0) && n.IntParamOr("BroadCastReception", 0) == 1 {
					n.RemoveAllAttackDesire()
					s.Attacked(s, script.Attacked{NPC: n, Attacker: c0, Damage: 1000})
				}
			}
			base.ScriptEvent(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			if s.Skill(e.Skill).AggroPoints() > 0 && int32(s.Rnd(100)) <= n.IntParamOr("ProbCond4", 30) {
				if len(e.Targets) > 0 && isPlayer(e.Targets[0]) {
					if s.Rnd(2) == 1 {
						addCastByType(n, e.Caster, npc.SkillTypePhysicalSpecial1, 10000000)
					}
					addCastByType(n, e.Caster, npc.SkillTypePhysicalSpecial2, 10000000)
				}
			}
			base.SeeSpell(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			n.SetInt(script.SlotIntAI4, s.CurrentTick())
			if c.Playable() {
				return
			}
			if n.IntParamOr("ag_type", 0) == 1 && isPlayer(c) {
				if !n.InCombat() {
					addCastByType(n, c, npc.SkillTypeLongRangeDDMagic1, 10000000)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}

// addCastByType makes n want to cast its template's skill of type t on
// target with weight; a template that lists none casts nothing.
func addCastByType(n *script.NPC, target script.Creature, t npc.SkillType, weight float64) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, weight)
	}
}
