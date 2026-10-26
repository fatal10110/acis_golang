package frintezza

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// UndeadBandmaster (18334), the dark choir captain. A Shoutman AI parameter
// of 1 makes it shout 1010644 as it appears, and 1010645 or 1010646 on
// script event arg1 10042 or 10043. Its ai slot 0 starts at 1; script event
// 10032 with arg1 0, 2, 4 or 6 sets it to arg1. Each hit draws against 33 in
// 100 to tell the NPCs within 1500 event 0, arg1 10033, arg2 the attacker's
// object id. Dying with the slot at 0, 2, 4 or 6 it drops one soul breaking
// arrow (8192) for its killer.
func UndeadBandmaster() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{18334}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.IntParam("Shoutman") == 1 {
				n.Shout(script.NpcString(1010644))
			}
			n.SetInt(script.SlotIntAI0, 1)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if s.Rnd(100) < 33 {
				s.BroadcastScriptEventEx(e.NPC, 0, 10033, e.Attacker.ObjectID(), 1500)
			}
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == 10032 && bandmasterArrowSlot(e.Arg1) {
				n.SetInt(script.SlotIntAI0, e.Arg1)
			}
			if n.IntParam("Shoutman") == 1 {
				switch e.Arg1 {
				case 10042:
					n.Shout(script.NpcString(1010645))
				case 10043:
					n.Shout(script.NpcString(1010646))
				}
			}
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			if bandmasterArrowSlot(e.NPC.Int(script.SlotIntAI0)) {
				e.NPC.DropItem(e.Killer, 8192, 1)
			}
		},
	})}
}

// bandmasterArrowSlot reports whether v is a bandmaster ai slot 0 value that
// drops the arrow: 0, 2, 4 or 6.
func bandmasterArrowSlot(v int32) bool {
	return v == 0 || v == 2 || v == 4 || v == 6
}
