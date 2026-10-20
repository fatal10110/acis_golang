package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// TyrannoPrimevalWalker is TyrannoPrimevalWalker: a TyrannoPrimeval that,
// once created, walks the route its SuperPointName parameter names, with
// weight 50.
func TyrannoPrimevalWalker() script.Script {
	base := tyrannoPrimevalHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/TyrannoPrimeval", Behavior: true, NPCs: []int32{22217}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			base.Created(s, e)
			route, _ := e.NPC.StringParam("SuperPointName")
			e.NPC.AddMoveRouteDesire(route, 50)
		},
	})}
}
