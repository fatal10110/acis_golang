package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// RunAwayPhysicalSpecialGolemAggressive is
// WarriorRunAwayPhysicalSpecialGolemAggressive: a golem Warrior that
// attacks the players and summons it sees, strikes from afar at what it
// fights once that is farther than 200, checking again every 5 seconds,
// and, first hit from closer than 100, runs the configured drift range
// away from its last attacker two seconds later.
//
// Its memory: IntAI1 is set while that run is pending, Creature1 is its
// last attacker.
func RunAwayPhysicalSpecialGolemAggressive() script.Script {
	base := WarriorHooks()
	longRange := func(n *script.NPC, target script.Creature) {
		addCastByType(n, target, npc.SkillTypeLongRangePhysicalSpecial, 1000000)
	}
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22050, 22056}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			if n.Distance2D(c) > 200 {
				longRange(n, c)
			}
			s.StartTimer("1002", n, nil, 5000*time.Millisecond)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetSlot(script.SlotCreature1, attacker)
			if n.Int(script.SlotIntAI1) == 0 && n.Distance2D(attacker) < 100 {
				n.SetInt(script.SlotIntAI1, 1)
				s.StartTimer("1003", n, nil, 2000*time.Millisecond)
			}
			if s.Rnd(100) < 33 {
				addCastByType(n, attacker, npc.SkillTypePhysicalSpecial, 1000000)
			}
			if n.Distance2D(attacker) > 200 {
				longRange(n, attacker)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1002"):
				if c1 := n.Slot(script.SlotCreature1); c1 != nil && n.Distance2D(c1) > 200 {
					longRange(n, c1)
				}
				s.StartTimer("1002", n, nil, 5000*time.Millisecond)
			case strings.EqualFold(e.Name, "1003"):
				n.AddFleeDesire(n.Slot(script.SlotCreature1), n.MaxDriftRange(), 1000)
				n.SetInt(script.SlotIntAI1, 0)
			}
			return ""
		},
	})}
}
