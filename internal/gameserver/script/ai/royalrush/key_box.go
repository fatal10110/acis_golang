package royalrush

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// KeyBox is RoyalRushKeyBox, the box a sepulcher room leaves for its
// Sepulcher Key: the first talk gives the talker the key (7260) once, and
// any talk takes the box out of the world, showing no page.
func KeyBox() script.Script {
	base := DefaultNpcHooks()
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs: []int32{31455, 31456, 31457, 31458, 31459, 31460, 31461, 31462, 31463, 31464, 31465, 31466, 31467},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				e.NPC.SetInt(script.SlotIntAI0, 0)

				base.Created(s, e)
			},
			OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
				n := e.NPC
				if n.Int(script.SlotIntAI0) == 0 {
					s.GiveItems(e.Player, 7260, 1)
					n.SetInt(script.SlotIntAI0, 1)
				}
				n.DeleteMe()
				return ""
			},
		}),
	}
}
