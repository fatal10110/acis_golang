package guard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// MoveAroundFixed is GuardMoveAroundFixed: a GuardMoveAround that, every 5
// minutes it is not attacking, teleports back to a point of its spawn.
func MoveAroundFixed() script.Script {
	base := MoveAroundHooks()
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs: []int32{31032, 31033, 31034, 31035, 31036},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				s.StartTimerAtFixedRate("5001", e.NPC, nil, 300000*time.Millisecond, 300000*time.Millisecond)

				base.Created(s, e)
			},
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				if strings.EqualFold(e.Name, "5001") {
					if n.Intention() != script.IntentionAttack {
						n.TeleportTo(s.Spawner().DrawSpawnLocation(n))
					}
				}
				return base.Timer(s, e)
			},
		}),
	}
}
