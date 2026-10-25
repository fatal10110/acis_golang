package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PhysicalSpecialVelociraptor is WarriorPhysicalSpecialVelociraptor: a
// WarriorPhysicalSpecialAggressive that, once told by script event 11049,
// sends the NPCs within 8000 script event 11051 whenever it has nothing to
// do and has neither been created, hit nor seen anyone for over 600
// ticks. With nothing to do it never wanders.
//
// Its memory: IntAI4 is the tick it was last created, hit or saw anyone;
// IntQuest0 is 1 once told.
func PhysicalSpecialVelociraptor() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22218}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, s.CurrentTick())
			e.NPC.SetInt(npc.IntQuest0, 0)
			base.Created(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == 11049 {
				e.NPC.SetInt(npc.IntQuest0, 1)
			}
			base.ScriptEvent(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			e.NPC.SetInt(script.SlotIntAI4, s.CurrentTick())
			base.Attacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			e.NPC.SetInt(script.SlotIntAI4, s.CurrentTick())
			base.SeeCreature(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.Int(npc.IntQuest0) == 1 && s.ElapsedTicks(n.Int(script.SlotIntAI4)) > 600 {
				s.BroadcastScriptEvent(n, 11051, 0, 8000)
			}
		},
	})}
}

// UsePowershotAggressive is WarriorUsePowershotAggressive: an archer
// WarriorPhysicalSpecialAggressive that, first hit from closer than 100,
// runs the configured drift range away from that attacker two seconds
// later.
//
// Its memory: IntAI2 is set while that run is pending, Creature1 is the
// attacker it runs from.
func UsePowershotAggressive() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{20812, 20660, 21008, 20826, 21064, 20623, 21642}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI2) == 0 && n.Distance2D(e.Attacker) < 100 {
				s.StartTimer("100002", n, nil, 2000*time.Millisecond)
				n.SetInt(script.SlotIntAI2, 1)
				n.SetSlot(script.SlotCreature1, e.Attacker)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "100002") {
				n := e.NPC
				n.AddFleeDesire(n.Slot(script.SlotCreature1), n.MaxDriftRange(), 10000)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
	})}
}
