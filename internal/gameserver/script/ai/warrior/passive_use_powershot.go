package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PassiveUsePowershot is WarriorPassiveUsePowershot: an archer
// WarriorPhysicalSpecial that, first hit from closer than 100, runs the
// configured drift range away from that attacker two seconds later.
//
// Its memory: IntAI2 is set while that run is pending, Creature1 is the
// attacker it runs from.
func PassiveUsePowershot() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: passiveUsePowershotIDs(), Hooks: base.With(script.Hooks{
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

// passiveUsePowershotIDs are WarriorPassiveUsePowershot's own NPCs.
func passiveUsePowershotIDs() []int32 {
	return []int32{
		20268, 20237, 20790, 20805, 21105, 21019, 21113, 21400, 20920, 20631, 20602, 21639, 20584, 21001,
		20833, 21605, 21606, 20224, 20256, 20855, 21013, 21440, 21607, 22019,
	}
}
