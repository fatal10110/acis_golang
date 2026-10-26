package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// AncientEgg is AncientEgg: a WarriorHold egg of the primeval isle that,
// most times it is hit, calls on everything within 300 to turn on its
// attacker (script event 10016 naming it). It neither fights back nor
// heeds script events itself.
func AncientEgg() script.Script {
	base := HoldHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorHold", Behavior: true, NPCs: []int32{18344}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if s.Rnd(100) <= 80 {
				s.BroadcastScriptEvent(e.NPC, eventShoutTarget, e.Attacker.ObjectID(), 300)
			}
		},
		OnScriptEvent: func(*script.Script, script.ScriptEvent) {},
	})}
}
