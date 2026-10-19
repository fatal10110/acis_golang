package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Disguise is WarriorDisguise: a monster in disguise. The first hit puts
// its silhouette where it stands, turning on the attacker, and takes it out
// of the world.
//
// Its memory: IntAI2 is set once it was hit.
func Disguise() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21266, 21267, 21258}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI2, 0)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI2) == 0 {
				s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette"), script.LocOf(n), 0, script.Params{P1: 1000, P2: e.Attacker.ObjectID(), P3: 0})
				n.SetInt(script.SlotIntAI2, 1)
				n.DeleteMe()
			}
		},
	})}
}
