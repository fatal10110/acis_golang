package sailren

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the script directory of the Sailren behaviors.
const dir = "ai/boss/sailren"

// slotLeader is the dummy's script memory: the object id of the leader of
// the party of the last player it saw in one.
const slotLeader = script.SlotIntAI0

// The two production skills the dummy casts on itself through the
// cutscene.
var (
	production  = skill.Ref{ID: 5090, Level: 1}
	production2 = skill.Ref{ID: 5091, Level: 1}
)

// SailrenDummy is the invisible NPC that stages Sailren's nest. It keeps
// the leader of the party of the last player it saw in one. Script event
// 11044 starts the cutscene 180006 ms later: six production casts on
// timers 1001 to 1006 and, from 4.1 s, six camera shots on timers 2000
// to 2005, after which timer 2006 tells the NPCs within 1000 event 11055
// with the leader it keeps. Events 11046, 11047 and 11048 send the players
// in its territory out of the nest; 11051 does too, then runs 11052 and
// 1000 on the three dinosaur makers and 11050 on the statue's maker.
// Event 11054, sent by a dying Sailren, leaves the teleport cube.
func SailrenDummy() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{32110}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(slotLeader, 0)

			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n, name := e.NPC, e.Name
			switch {
			case strings.EqualFold(name, "1001"), strings.EqualFold(name, "1002"), strings.EqualFold(name, "1003"), strings.EqualFold(name, "1004"):
				n.ClearDesires()

				n.AddCastDesire(n, production, 1000000)
			case strings.EqualFold(name, "1005"), strings.EqualFold(name, "1006"):
				n.ClearDesires()

				n.AddCastDesire(n, production2, 1000000)
			case strings.EqualFold(name, "2000"):
				n.SpecialCamera(script.CameraShot{Distance: 100, Yaw: 180, Pitch: 30, Time: 3000, Duration: 20000, Turn: 0, Rise: 50, Widescreen: 1, Unknown: 0})

				s.StartTimer("2001", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(name, "2001"):
				n.SpecialCamera(script.CameraShot{Distance: 150, Yaw: 270, Pitch: 25, Time: 3000, Duration: 20000, Turn: 0, Rise: 30, Widescreen: 1, Unknown: 0})

				s.StartTimer("2002", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(name, "2002"):
				n.SpecialCamera(script.CameraShot{Distance: 160, Yaw: 360, Pitch: 20, Time: 3000, Duration: 20000, Turn: 10, Rise: 15, Widescreen: 1, Unknown: 0})

				s.StartTimer("2003", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(name, "2003"):
				n.SpecialCamera(script.CameraShot{Distance: 160, Yaw: 450, Pitch: 10, Time: 3000, Duration: 20000, Turn: 0, Rise: 10, Widescreen: 1, Unknown: 0})

				s.StartTimer("2004", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(name, "2004"):
				n.SpecialCamera(script.CameraShot{Distance: 160, Yaw: 560, Pitch: 0, Time: 3000, Duration: 20000, Turn: 0, Rise: 10, Widescreen: 1, Unknown: 0})

				s.StartTimer("2005", n, nil, 7000*time.Millisecond)
			case strings.EqualFold(name, "2005"):
				n.SpecialCamera(script.CameraShot{Distance: 70, Yaw: 560, Pitch: 0, Time: 500, Duration: 7000, Turn: -15, Rise: 20, Widescreen: 1, Unknown: 0})

				s.StartTimer("2006", n, nil, 15000*time.Millisecond)
			case strings.EqualFold(name, "2006"):
				s.BroadcastScriptEventEx(n, 11055, n.Int(slotLeader), 0, 1000)
			case strings.EqualFold(name, "3001"):
				n.SpecialCamera(script.CameraShot{Distance: 60, Yaw: 110, Pitch: 30, Time: 4000, Duration: 20000, Turn: 0, Rise: 65, Widescreen: 1, Unknown: 0})
				n.AddCastDesire(n, production, 1000000)
				s.StartTimer("1001", n, nil, 3400*time.Millisecond)
				n.AddCastDesire(n, production, 1000000)
				s.StartTimer("1002", n, nil, 6800*time.Millisecond)
				n.AddCastDesire(n, production, 1000000)
				s.StartTimer("1003", n, nil, 10000*time.Millisecond)
				n.AddCastDesire(n, production, 1000000)
				s.StartTimer("1004", n, nil, 13200*time.Millisecond)
				n.AddCastDesire(n, production2, 1000000)
				s.StartTimer("1005", n, nil, 16500*time.Millisecond)
				n.AddCastDesire(n, production2, 1000000)
				s.StartTimer("1006", n, nil, 23500*time.Millisecond)
				s.StartTimer("2000", n, nil, 4100*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if p, ok := e.Creature.(*script.Player); ok {
				if leader := p.PartyLeader(); leader != nil {
					e.NPC.SetInt(slotLeader, leader.ObjectID())
				}
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 11044:
				s.StartTimer("3001", n, nil, 180006*time.Millisecond)
			case 11046, 11048:
				oustPlayers(s, n)
			case 11051:
				oustPlayers(s, n)

				for _, maker := range []string{"rune20_mb2017_01m1", "rune20_mb2017_02m1", "rune20_mb2017_03m1"} {
					s.Spawner().SendMakerEvent(maker, "11052", 0, 0)
					s.Spawner().SendMakerEvent(maker, "1000", 0, 0)
				}

				s.Spawner().SendMakerEvent("rune16_npc2017_01m1", "11050", 0, 0)
			case 11054:
				s.Spawner().CreateOnePrivateEx(n, 32107, script.Loc{X: 27644, Y: -6638, Z: -2008, Heading: 0}, 0, script.Params{})
			case 11047:
				oustPlayers(s, n)
			}
		},
	})}
}

// oustPlayers sends every player in the territory of the dummy's spawn to
// one of the nest's two exits, chosen at even odds, up to 100 away on each
// axis.
func oustPlayers(s *script.Script, n *script.NPC) {
	if s.Rnd(100) < 50 {
		s.TeleportPlayersInTerritory(n, script.Loc{X: 23575, Y: -7727, Z: -1272}, 100)
	} else {
		s.TeleportPlayersInTerritory(n, script.Loc{X: 23421, Y: -8167, Z: -1326}, 100)
	}
}
