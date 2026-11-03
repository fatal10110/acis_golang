package royalrush

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// triggerDir is the page directory of the trigger boxes.
const triggerDir = "ai/individual/RoyalRushDefaultNpc/RoyalRushTriggerBoxBase"

// TriggerBoxBase is RoyalRushTriggerBoxBase. It binds no NPC of its own.
func TriggerBoxBase() script.Script {
	return script.Script{Dir: triggerDir, Behavior: true, Hooks: TriggerBoxBaseHooks()}
}

// TriggerBoxBaseHooks are RoyalRushTriggerBoxBase's hooks, which the
// trigger boxes derive theirs from: a trigger box, unlike the other Royal
// Rush NPCs, stays in the world past minute 54, and a talk takes it out of
// the world, showing no page.
func TriggerBoxBaseHooks() script.Hooks {
	base := DefaultNpcHooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
		},
		OnFirstTalk: func(_ *script.Script, e script.FirstTalk) string {
			e.NPC.DeleteMe()
			return ""
		},
	}).Named("RoyalRushTriggerBoxBase")
}

// armTrigger reports whether a talk to the trigger box n starts a room:
// in minutes 50 to 59 the box says the rooms are closed and stays, and a
// box already started stays silent. A box it reports true for is started.
func armTrigger(s *script.Script, n *script.NPC) bool {
	if i0 := s.Now().Minute(); i0 > 49 && i0 < 60 {
		n.Say(script.NpcString(1010552))
		return false
	}
	if n.Int(script.SlotIntAI0) == 1 {
		return false
	}
	n.SetInt(script.SlotIntAI0, 1)
	return true
}

// startRoom runs maker script events 1000 and 1001 on the npcmaker the
// trigger box n's AI parameter param names, def when it names none: the
// room's NPCs leave the world and the room fills again.
func startRoom(s *script.Script, n *script.NPC, param, def string) {
	room, ok := n.StringParam(param)
	if !ok {
		room = def
	}
	s.Spawner().SendMakerEvent(room, "1000", 0, 0)
	s.Spawner().SendMakerEvent(room, "1001", 0, 0)
}

// TriggerBox1 is RoyalRushTriggerBox1, the trigger box of a sepulcher's
// first room: a talk starts one of its three rooms on a draw of 3.
func TriggerBox1() script.Script {
	base := TriggerBoxBaseHooks()
	return script.Script{Dir: triggerDir, Behavior: true, NPCs: []int32{31468, 31473, 31478, 31483}, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			n := e.NPC
			if !armTrigger(s, n) {
				return ""
			}
			switch s.Rnd(3) {
			case 0:
				startRoom(s, n, "room_event_1", "1rd_type1_a")
			case 1:
				startRoom(s, n, "room_event_2", "1rd_type2_a")
			case 2:
				startRoom(s, n, "room_event_3", "1rd_type3_a")
			}
			return base.FirstTalk(s, e)
		},
	})}
}

// TriggerBox2 is RoyalRushTriggerBox2, the trigger box of a sepulcher's
// second room: a talk starts one of its two rooms on a draw of 2.
func TriggerBox2() script.Script {
	base := TriggerBoxBaseHooks()
	return script.Script{Dir: triggerDir, Behavior: true, NPCs: []int32{31469, 31474, 31479, 31484}, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			n := e.NPC
			if !armTrigger(s, n) {
				return ""
			}
			switch s.Rnd(2) {
			case 0:
				startRoom(s, n, "room_event_1", "1rd_type1_b1")
			case 1:
				startRoom(s, n, "room_event_2", "1rd_type2_b1")
			}
			return base.FirstTalk(s, e)
		},
	})}
}

// TriggerBox3 is RoyalRushTriggerBox3, the trigger box of a sepulcher's
// third room: a talk starts one of its two rooms on a draw of 2.
func TriggerBox3() script.Script {
	base := TriggerBoxBaseHooks()
	return script.Script{Dir: triggerDir, Behavior: true, NPCs: []int32{31470, 31475, 31480, 31485}, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			n := e.NPC
			if !armTrigger(s, n) {
				return ""
			}
			switch s.Rnd(2) {
			case 0:
				startRoom(s, n, "room_event_1", "1rd_type1_c")
			case 1:
				startRoom(s, n, "room_event_2", "1rd_type2_c")
			}
			return base.FirstTalk(s, e)
		},
	})}
}

// TriggerBox4 is RoyalRushTriggerBox4, the trigger box of a sepulcher's
// fourth room: a talk starts one of its three rooms on a draw of 3.
func TriggerBox4() script.Script {
	base := TriggerBoxBaseHooks()
	return script.Script{Dir: triggerDir, Behavior: true, NPCs: []int32{31471, 31476, 31481, 31486}, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			n := e.NPC
			if !armTrigger(s, n) {
				return ""
			}
			switch s.Rnd(3) {
			case 0:
				startRoom(s, n, "room_event_1", "1rd_type1_d")
			case 1:
				startRoom(s, n, "room_event_2", "1rd_type2_d")
			case 2:
				startRoom(s, n, "room_event_3", "1rd_type3_d")
			}
			return base.FirstTalk(s, e)
		},
	})}
}

// TriggerBox5 is RoyalRushTriggerBox5, the trigger box of a sepulcher's
// fifth room: a talk starts one of its two rooms on a draw of 2.
func TriggerBox5() script.Script {
	base := TriggerBoxBaseHooks()
	return script.Script{Dir: triggerDir, Behavior: true, NPCs: []int32{31472, 31477, 31482, 31487}, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			n := e.NPC
			if !armTrigger(s, n) {
				return ""
			}
			switch s.Rnd(2) {
			case 0:
				startRoom(s, n, "room_event_1", "1rd_type1_e")
			case 1:
				startRoom(s, n, "room_event_2", "1rd_type2_e")
			}
			return base.FirstTalk(s, e)
		},
	})}
}
