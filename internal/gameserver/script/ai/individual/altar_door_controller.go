package individual

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The npcmakers the altar door keeper drives.
const (
	makerUniqueNPC2ndFloor = "altar_start_02"
	makerJako2ndFloor      = "altar_start_01"
	makerStageCon          = "rune14_andreas_m2"
)

// AltarDoorController is the altar door keeper. It keeps the stage of the
// altar in its first int slot: the script event that set it, below 9. Each
// stage opens or closes its doors, the one of its DoorName AI parameter and
// the third floor one of DoorName3rdFloor, sends the makers of the altar
// their events and arms the timer that moves the altar on; a creature it
// sees at stage 0 or 2 starts the next stage around it. Talking to it shows
// nothing.
func AltarDoorController() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{32051}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
		},
		OnFirstTalk: func(*script.Script, script.FirstTalk) string {
			return ""
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			switch e.NPC.Int(script.SlotIntAI0) {
			case 0:
				s.BroadcastScriptEvent(e.NPC, 1, e.Creature.ObjectID(), 4000)
			case 2:
				s.BroadcastScriptEvent(e.NPC, 3, e.Creature.ObjectID(), 4000)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			doorName, hasDoor := n.StringParam("DoorName")
			doorName3rdFloor, hasDoor3rdFloor := n.StringParam("DoorName3rdFloor")
			switch e.EventID {
			case 0:
				if hasDoor {
					defaultnpc.OpenCloseDoor(s, doorName, 0)
				}
				if hasDoor3rdFloor {
					defaultnpc.OpenCloseDoor(s, doorName3rdFloor, 1)
				}
			case 1:
				s.Spawner().SendMakerEvent(makerUniqueNPC2ndFloor, "1001", 0, 0)
				s.Spawner().SendMakerEvent(makerJako2ndFloor, "1001", 0, 0)
				s.Spawner().SendMakerEvent(makerStageCon, "1001", 0, 0)
				s.StartTimer("2001", n, nil, 3*60*1000*time.Millisecond)
			case 2:
				n.Shout(script.NpcString(10079))
				if hasDoor3rdFloor {
					defaultnpc.OpenCloseDoor(s, doorName3rdFloor, 0)
				}
				s.StartTimer("2002", n, nil, 3*60*1000*time.Millisecond)
			case 3:
				n.SetInt(script.SlotIntAI1, 1)
				s.StartTimer("2003", n, nil, 2*60*1000*time.Millisecond)
			case 5:
				if hasDoor3rdFloor {
					defaultnpc.OpenCloseDoor(s, doorName3rdFloor, 0)
				}
				if hasDoor {
					defaultnpc.OpenCloseDoor(s, doorName, 0)
				}
				s.Spawner().SendMakerEvent(makerStageCon, "1000", 0, 0)
				s.StartTimer("2004", n, nil, 3*60*1000*time.Millisecond)
			case 6:
				if hasDoor3rdFloor {
					defaultnpc.OpenCloseDoor(s, doorName3rdFloor, 1)
				}
				s.StartTimer("2005", n, nil, 3*60*1000*time.Millisecond)
			case 7:
				if hasDoor3rdFloor {
					defaultnpc.OpenCloseDoor(s, doorName3rdFloor, 0)
				}
				s.StartTimer("2006", n, nil, 60*60*1000*time.Millisecond)
			}
			if e.EventID < 9 {
				n.SetInt(script.SlotIntAI0, e.EventID)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			doorName, hasDoor := n.StringParam("DoorName")
			doorName3rdFloor, hasDoor3rdFloor := n.StringParam("DoorName3rdFloor")
			switch {
			case strings.EqualFold(e.Name, "2001"):
				if hasDoor {
					defaultnpc.OpenCloseDoor(s, doorName, 1)
				}
			case strings.EqualFold(e.Name, "2002"):
				if n.Int(script.SlotIntAI1) == 0 && hasDoor3rdFloor {
					defaultnpc.OpenCloseDoor(s, doorName3rdFloor, 1)
				}
			case strings.EqualFold(e.Name, "2003"):
				n.SetInt(script.SlotIntAI1, 0)
				s.StartTimer("2002", n, e.Player, 60000*time.Millisecond)
			case strings.EqualFold(e.Name, "2004"):
				s.BroadcastScriptEvent(n, 6, n.ObjectID(), 4000)
			case strings.EqualFold(e.Name, "2005"):
				n.DeleteMe()
			case strings.EqualFold(e.Name, "2006"):
				s.Spawner().SendMakerEvent(makerUniqueNPC2ndFloor, "1001", 0, 0)
				s.Spawner().SendMakerEvent(makerStageCon, "1001", 0, 0)
				if hasDoor {
					defaultnpc.OpenCloseDoor(s, doorName, 0)
				}
				n.SetInt(script.SlotIntAI0, 0)
				s.BroadcastScriptEvent(n, 0, n.ObjectID(), 4000)
			}
			return base.Timer(s, e)
		},
	})}
}
