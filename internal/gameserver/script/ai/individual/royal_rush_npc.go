package individual

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// RoyalRushNpc is the manager of a Four Sepulchers sepulcher. Created, it
// places the five lock NPCs of its lock_* AI parameters as its privates
// (at the Conquerors' Sepulcher doors, numbered 1 to 5, when lock_x1 is 0)
// and reads the local clock every second: in the first 5 s of each hour
// it clears the room_trigger_1 npcmaker (event 1000) and refills it 5 s
// later (1001); at minutes 5 to 45 of each five and second 0 it shouts the
// minutes gone, at minute 50 that the entry closes, and sends the players
// of its territory to its EscapeTelPos; in the first 30 s of minute 54 it
// frees the sepulcher (AVQuest0 slot 0); at minute 55 it shouts the two
// closing lines. ShoutMsg 1 turns on the minute shouts. Script events 1001
// to 1004 reset rooms 2 to 5, and 1005 one of the boss rooms, each npcmaker
// cleared and refilled.
func RoyalRushNpc() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: "ai/individual/RoyalRushNpc", Behavior: true,
		NPCs: []int32{31921, 31922, 31923, 31924},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				s.StartTimerAtFixedRate("3000", n, nil, 1000*time.Millisecond, 1000*time.Millisecond)
				n.SetInt(script.SlotIntAI0, 0)
				n.SetInt(script.SlotIntAI1, 0)

				if n.IntParam("lock_x1") != 0 {
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id1"), lockAt(n, "lock_x1", "lock_y1", "lock_z1", "lock_d1"), 0, script.Params{})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id2"), lockAt(n, "lock_x2", "lock_y2", "lock_z2", "lock_d2"), 0, script.Params{})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id3"), lockAt(n, "lock_x3", "lock_y3", "lock_z3", "lock_d3"), 0, script.Params{})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id4"), lockAt(n, "lock_x4", "lock_y4", "lock_z4", "lock_d4"), 0, script.Params{})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id5"), lockAt(n, "lock_x5", "lock_y5", "lock_z5", "lock_d5"), 0, script.Params{})
				} else {
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id1"), script.Loc{X: 182727, Y: -85493, Z: -7200, Heading: -32584}, 0, script.Params{P1: 1, P2: 0, P3: 0})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id2"), script.Loc{X: 184547, Y: -85479, Z: -7200, Heading: -32584}, 0, script.Params{P1: 2, P2: 0, P3: 0})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id3"), script.Loc{X: 186349, Y: -85473, Z: -7200, Heading: -32584}, 0, script.Params{P1: 3, P2: 0, P3: 0})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id4"), script.Loc{X: 188154, Y: -85463, Z: -7200, Heading: -32584}, 0, script.Params{P1: 4, P2: 0, P3: 0})
					s.Spawner().CreateOnePrivateEx(n, n.IntParam("lock_npc_id5"), script.Loc{X: 189947, Y: -85466, Z: -7200, Heading: -32584}, 0, script.Params{P1: 5, P2: 0, P3: 0})
				}
			},
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				switch {
				case strings.EqualFold(e.Name, "3001"):
					if n.Int(script.SlotIntAI1) == 1 {
						room, _ := n.StringParam("room_trigger_1")
						s.Spawner().SendMakerEvent(room, "1001", 0, 0)

						n.SetInt(script.SlotIntAI1, 0)
					}
				case strings.EqualFold(e.Name, "3000"):
					now := s.Now()
					i0, i1 := now.Minute(), now.Second()
					shoutMsg := n.IntParam("ShoutMsg")
					switch {
					case i0 == 0 && i1 < 5:
						if n.Int(script.SlotIntAI1) == 0 {
							room, _ := n.StringParam("room_trigger_1")
							s.Spawner().SendMakerEvent(room, "1000", 0, 0)

							n.SetInt(script.SlotIntAI1, 1)
							s.StartTimer("3001", n, nil, 5000*time.Millisecond)
						}
					case (i0 == 5 || i0 == 10 || i0 == 15 || i0 == 20 || i0 == 25 || i0 == 30 || i0 == 35 || i0 == 40 || i0 == 45) && i1 == 0:
						if shoutMsg == 1 {
							n.ShoutInRadius(script.NpcString(1000455)+strconv.Itoa(i0)+script.NpcString(1000456), 11500)
						}
					case i0 == 50 && i1 == 0:
						if shoutMsg == 1 {
							n.ShoutInRadius(script.NpcString(1000457), 11500)
						}

						escapeTelPosX := n.IntParam("EscapeTelPosX")
						escapeTelPosY := n.IntParam("EscapeTelPosY")
						escapeTelPosZ := n.IntParam("EscapeTelPosZ")
						if escapeTelPosX != 0 && escapeTelPosY != 0 && escapeTelPosZ != 0 {
							s.TeleportPlayersInTerritory(n, script.Loc{X: int(escapeTelPosX), Y: int(escapeTelPosY), Z: int(escapeTelPosZ)}, 100)
						}
					case i0 == 54 && i1 >= 0 && i1 <= 30:
						n.SetInt(npc.IntAVQuest0, 0)
					case i0 == 55 && i1 == 0:
						n.ShoutInRadius(script.NpcString(1000500), 11500)
						n.ShoutInRadius(script.NpcString(1000501), 11500)
					}
				}
				return ""
			},
			OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
				n := e.NPC
				var room string
				switch e.EventID {
				case 1001:
					room, _ = n.StringParam("room_trigger_2")
				case 1002:
					room, _ = n.StringParam("room_trigger_3")
				case 1003:
					room, _ = n.StringParam("room_trigger_4")
				case 1004:
					room, _ = n.StringParam("room_trigger_5")
				case 1005:
					i0 := s.Rnd(4) + 1
					boss, _ := n.StringParam("room_trigger_boss")
					room = boss + "_type" + strconv.Itoa(i0)
				default:
					return
				}
				resetRoom(s, room)
			},
		}),
	}
}

// lockAt is where the AI parameters x, y, z and d of n place a lock NPC,
// facing heading d.
func lockAt(n *script.NPC, x, y, z, d string) script.Loc {
	return script.Loc{X: int(n.IntParam(x)), Y: int(n.IntParam(y)), Z: int(n.IntParam(z)), Heading: int(n.IntParam(d))}
}

// resetRoom clears the npcmaker named room (event 1000) and refills it
// (1001); nothing runs when no npcmaker has that name.
func resetRoom(s *script.Script, room string) {
	if s.Spawner().SendMakerEvent(room, "1000", 0, 0) {
		s.Spawner().SendMakerEvent(room, "1001", 0, 0)
	}
}
