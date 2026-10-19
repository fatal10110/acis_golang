package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// HereticSelfRangePhysical is WarriorHereticSelfRangePhysical: a
// WarriorCastingCurse that keeps to its territory until it is drawn into a
// fight. Hit, or told of a hit on its clan, while neither attacking nor
// casting, it gives up the territory (i_ai4); out of the territory before
// that it drops its attack desires and walks back to its spawn. It may
// cast its self range physical skill on the attacker it last took up.
// Script events 10002 and 10033 turn it on the creature they name (10033
// also releases it from the territory), and 10035 sends it home bound to
// the territory again.
func HereticSelfRangePhysical() script.Script {
	return script.Script{Dir: castingCurseDir, Behavior: true, NPCs: []int32{22147, 22152}, Hooks: HereticSelfRangePhysicalHooks()}
}

// HereticSelfRangePhysicalHooks are WarriorHereticSelfRangePhysical's
// hooks, which its children derive theirs from.
func HereticSelfRangePhysicalHooks() script.Hooks {
	base := CastingCurseHooks()
	goHome := func(n *script.NPC) {
		n.RemoveAllAttackDesire()
		n.AddMoveToDesire(n.SpawnLocation(), 100)
	}
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if !fighting(n.Intention()) && n.Int(script.SlotIntAI4) == 0 {
				n.SetInt(script.SlotIntAI4, 1)
			}
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil && s.Rnd(100) < 33 && script.Same(top, attacker) {
					addCastByType(n, attacker, npc.SkillTypeSelfRangePhysicalSpecial, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if !fighting(called.Intention()) && called.Int(script.SlotIntAI4) == 0 {
				called.SetInt(script.SlotIntAI4, 1)
			}
			if called.LifeTime() > 7 && attacker.Playable() {
				if top := called.TopDesireTarget(); top != nil && s.Rnd(100) < 33 && script.Same(top, attacker) {
					addCastByType(called, attacker, npc.SkillTypeSelfRangePhysicalSpecial, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
			if e.NPC.Int(script.SlotIntAI4) == 0 {
				goHome(e.NPC)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 10033, 10002:
				c0 := s.Object(e.Arg1)
				if c0 == nil {
					return
				}
				if e.EventID == 10033 {
					n.SetInt(script.SlotIntAI4, 1)
				}
				n.RemoveAllAttackDesire()
				if c0.Playable() {
					n.AddAttackDesire(c0, 200)
				}
			case 10035:
				n.SetInt(script.SlotIntAI4, 0)
				goHome(n)
			}
		},
	}).Named("WarriorHereticSelfRangePhysical")
}

// HereticSelfRangePhysicalAggressive is
// WarriorHereticSelfRangePhysicalAggressive: a
// WarriorHereticSelfRangePhysical that attacks the players and summons it
// sees.
func HereticSelfRangePhysicalAggressive() script.Script {
	base := HereticSelfRangePhysicalHooks()
	return script.Script{Dir: castingCurseDir, Behavior: true, NPCs: []int32{22141}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if e.Creature.Playable() {
				defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			}
			base.SeeCreature(s, e)
		},
	})}
}
