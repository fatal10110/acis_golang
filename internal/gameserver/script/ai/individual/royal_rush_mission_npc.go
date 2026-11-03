package individual

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// RoyalRushMissionNpc is the victim of a Royal Rush mission room. It
// follows the players it sees and calls for help every 5 s while its first
// int slot is 0; after 5 minutes alive it drops the key box of its KeyBox
// AI parameter, sets the slot to 1 and casts one of its four skills at
// each player it sees. Killed before that, it leaves the NPC of its
// StrongNPC AI parameter in its place. It flees whoever hits it, and tells
// most NPCs it sees (event 1234) that it is there.
func RoyalRushMissionNpc() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs: []int32{18150, 18151, 18152, 18153, 18154, 18155, 18156, 18157},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				s.StartTimer("3000", n, nil, 300000*time.Millisecond)
				s.StartTimerAtFixedRate("3001", n, nil, 5000*time.Millisecond, 5000*time.Millisecond)

				n.LookNeighbor()
				n.SetInt(script.SlotIntAI0, 0)
			},
			OnNoDesire: func(_ *script.Script, e script.NoDesire) {
				e.NPC.AddWanderDesire(5, 5)
			},
			OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
				n, c := e.NPC, e.Creature
				if !c.Playable() {
					if s.Rnd(100) < 80 {
						s.SendScriptEvent(c.(*script.NPC), 1234, n.ObjectID(), 0)
					}
					return
				}
				n.AddFollowDesire(c, 100)

				if n.Int(script.SlotIntAI0) == 0 {
					n.Say(script.NpcString(1010483, c.Name()))
					return
				}
				switch s.Rnd(4) {
				case 0:
					n.AddCastDesire(c, skill.Ref{ID: 4384, Level: 1}, 1000000)
				case 1:
					n.AddCastDesire(c, skill.Ref{ID: 4385, Level: 1}, 1000000)
				case 2:
					n.AddCastDesire(c, skill.Ref{ID: 4386, Level: 1}, 1000000)
				case 3:
					n.AddCastDesire(c, skill.Ref{ID: 4387, Level: 1}, 1000000)
				}
			},
			OnAttacked: func(s *script.Script, e script.Attacked) {
				e.NPC.AddFleeDesire(e.Attacker, e.NPC.MaxDriftRange(), 100)

				base.Attacked(s, e)
			},
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				switch {
				case strings.EqualFold(e.Name, "3000"):
					if !n.Dead() {
						s.Spawner().CreateOnePrivateEx(n, n.IntParam("KeyBox"), here(n), 0, script.Params{})

						n.SetInt(script.SlotIntAI0, 1)
						n.Say(script.NpcString(1000503))
						n.LookNeighbor()
					}
				case strings.EqualFold(e.Name, "3001"):
					n.LookNeighbor()

					if n.Int(script.SlotIntAI0) == 0 {
						n.Say(script.NpcString(1010484))
					}
				}
				return ""
			},
			OnMyDying: func(s *script.Script, e script.MyDying) {
				n := e.NPC
				if n.Int(script.SlotIntAI0) == 0 {
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("StrongNPC"), here(n), 0, script.Params{})
				}
			},
		}),
	}
}

// here is where n stands, facing heading 0.
func here(n *script.NPC) script.Loc {
	at := script.LocOf(n)
	return script.Loc{X: at.X, Y: at.Y, Z: at.Z}
}
