package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyLeaderDD2Summon is PartyLeaderWizardDD2Summon: a PartyLeaderWizardDD2
// that, on its first hit, calls its three silhouettes around itself, each
// told to attack the attacker.
func PartyLeaderDD2Summon() script.Script {
	base := partyLeaderDD2Hooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{22033, 22035, 22041, 22043}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n := e.NPC
			if n.Int(script.SlotIntAI1) != 0 {
				return
			}
			at := script.LocOf(n)
			params := script.Params{P1: 1000, P2: e.Attacker.ObjectID()}
			i1 := s.Rnd(50) - 25
			i2 := s.Rnd(50) - 25
			s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette"), script.Loc{X: at.X + i1, Y: at.Y + i2, Z: at.Z}, 0, params)
			i1 = s.Rnd(100) - 50
			i2 = s.Rnd(100) - 50
			s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette2"), script.Loc{X: at.X + i1, Y: at.Y + i2, Z: at.Z}, 0, params)
			i1 = s.Rnd(100) - 50
			i2 = s.Rnd(100) - 50
			s.Spawner().CreateOnePrivateEx(n, n.IntParam("silhouette3"), script.Loc{X: at.X + i1, Y: at.Y + i2, Z: at.Z}, 0, params)
			n.SetInt(script.SlotIntAI1, 1)
		},
	})}
}
