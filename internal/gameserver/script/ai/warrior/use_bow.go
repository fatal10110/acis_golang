package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// UseBow is WarriorUseBow: an archer Warrior that, first hit from closer
// than 100, runs the configured drift range away from that attacker two
// seconds later.
//
// Its memory: IntAI2 is set while that run is pending, Creature1 is the
// attacker it runs from.
func UseBow() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: useBowIDs(), Hooks: base.With(script.Hooks{
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
				n.AddFleeDesire(n.Slot(script.SlotCreature1), n.MaxDriftRange(), 100000)
				n.SetInt(script.SlotIntAI2, 0)
			}
			return base.Timer(s, e)
		},
	})}
}

// useBowIDs are WarriorUseBow's own NPCs.
func useBowIDs() []int32 {
	return []int32{
		20011, 20094, 20296, 20423, 20614, 21133, 20385, 20273, 20463, 27060, 20469, 20361, 20578, 27125,
		20053, 20063, 20209, 20006, 20045, 20518, 20102, 20051, 20784, 20496, 20447, 22003, 22031, 22040,
	}
}

// UseBowNotFleeAggressive is WarriorUseBowNotFleeAggressive: an archer
// WarriorAggressive that stands its ground, healing itself below a third
// of its HP, or a clan member below a third of its HP that calls it, and
// now and then shooting its long range strike at the attacker it fights.
func UseBowNotFleeAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22132}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.HPRatio() < 0.33 {
				addCastByType(n, n, npc.SkillTypeMagicHeal, 1000000)
			}
			if s.Rnd(100) < 33 && script.Same(n.TopDesireTarget(), e.Attacker) {
				addCastByType(n, e.Attacker, npc.SkillTypeLongRangePhysicalSpecial, 1000000)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Caller.HPRatio() < 0.33 {
				addCastByType(e.Called, e.Caller, npc.SkillTypeMagicHeal, 1000000)
			}
			base.ClanAttacked(s, e)
		},
	})}
}
