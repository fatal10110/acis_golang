package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const primevalRoamerDir = "ai/individual/Monster/WarriorBase/Warrior/PartyLeaderWarrior/PrimevalRoamer"

// PrimevalRoamer is PrimevalRoamer: a PartyLeaderWarrior of the Primeval
// Isle that doubles its skill chances once low on HP and, lower still,
// drops its target once to buff those around it, then attacks that target
// again. Every hit may call those around it and strike the creature it
// most wants to attack with its physical special skills. With ag_type set
// it attacks the players it sees, a physical special skill first on a
// draw. Told by event 10016 of a player to fight, with BroadCastReception
// set it answers as to that player's hit. Left with nothing to do out of
// combat it resets its chances and, standing type, walks home.
//
// Its memory: IntAI2 is set once its buff landed, IntAI3 is the multiplier
// of its skill chances; Creature1 is the target it dropped to buff.
func PrimevalRoamer() script.Script {
	return script.Script{Dir: primevalRoamerDir, Behavior: true, NPCs: []int32{22200, 22224, 22203, 22225, 22196, 22208, 22226, 22223, 22211, 22227}, Hooks: primevalRoamerHooks()}
}

// primevalRoamerHooks are PrimevalRoamer's hooks, which its child derives
// its own from.
func primevalRoamerHooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI2, 0)
			n.SetInt(script.SlotIntAI3, 1)
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
			base.NoDesire(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			topDesireTarget := n.TopDesireTarget()
			if n.HPRatio()*100 <= float64(n.IntParamOr("HpChkRate4", 50)) {
				n.SetInt(script.SlotIntAI3, 2)
			} else {
				n.SetInt(script.SlotIntAI3, 1)
			}
			if n.HPRatio()*100 <= float64(n.IntParamOr("HpChkRate5", 30)) {
				if n.Int(script.SlotIntAI2) == 0 {
					if topDesireTarget != nil {
						n.SetSlot(script.SlotCreature1, topDesireTarget)
					}
					n.RemoveAllAttackDesire()
					addCastByType(n, n, npc.SkillTypeSelfRangeBuff1, 10000000)
				}
			}
			if int32(s.Rnd(100)) <= n.IntParamOr("ProbCond5", 20)*n.Int(script.SlotIntAI3) {
				s.BroadcastScriptEvent(n, 10002, n.ObjectID(), int(n.IntParamOr("BroadCastRange", 300)))
			}
			if topDesireTarget != nil {
				if int32(s.Rnd(100)) <= n.IntParam("ProbPhysicalSpecial1")*n.Int(script.SlotIntAI3) {
					addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 1000000)
				}
				if int32(s.Rnd(100)) <= n.IntParam("ProbPhysicalSpecial2")*n.Int(script.SlotIntAI3) {
					addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial2, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if isSkillOfType(n, e.Skill, npc.SkillTypeSelfRangeBuff1) && e.Success {
				n.SetInt(script.SlotIntAI2, 1)
				resumeDroppedTarget(n)
			}
			base.UseSkillFinished(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == eventShoutTarget {
				if c0 := s.Object(e.Arg1); isPlayer(c0) && n.IntParam("BroadCastReception") == 1 {
					n.RemoveAllAttackDesire()
					s.Attacked(s, script.Attacked{NPC: n, Attacker: c0, Damage: 1000})
				}
			}
			base.ScriptEvent(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if isPlayer(c) && n.IntParam("ag_type") == 1 {
				if int32(s.Rnd(100)) <= n.IntParam("ProbPhysicalSpecial1")*n.Int(script.SlotIntAI3) {
					addCastByType(n, c, npc.SkillTypePhysicalSpecial1, 1000000)
				} else if int32(s.Rnd(100)) <= n.IntParam("ProbPhysicalSpecial2")*n.Int(script.SlotIntAI3) {
					addCastByType(n, c, npc.SkillTypePhysicalSpecial2, 1000000)
				}
				defaultnpc.TryToAttack(s, n, c)
				base.SeeCreature(s, e)
			}
		},
	}).Named("PrimevalRoamer")
}

// PrimevalRoamerWalker is PrimevalRoamerWalker: a PrimevalRoamer that,
// once created, walks the route its SuperPointName parameter names, with
// weight 50.
func PrimevalRoamerWalker() script.Script {
	base := primevalRoamerHooks()
	return script.Script{Dir: primevalRoamerDir, Behavior: true, NPCs: []int32{22205, 22202, 22210, 22198, 22213}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			base.Created(s, e)
			route, _ := e.NPC.StringParam("SuperPointName")
			e.NPC.AddMoveRouteDesire(route, 50)
		},
	})}
}
