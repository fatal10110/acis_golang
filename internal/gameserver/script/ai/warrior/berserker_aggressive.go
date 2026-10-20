package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// BerserkerAggressive is WarriorBerserkerAggressive: a WarriorAggressive
// that roars as its clan members die, casting its fury on itself one level
// higher each time, up to its MaxRoarCount.
func BerserkerAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{21269, 21271}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnClanDied: func(s *script.Script, e script.ClanDied) {
			if called := e.Called; !script.Same(e.Caller, called) {
				if fury, ok := roar(called); ok {
					called.AddCastDesire(called, fury, 1000000)
				}
			}
			base.ClanDied(s, e)
		},
	})}
}
