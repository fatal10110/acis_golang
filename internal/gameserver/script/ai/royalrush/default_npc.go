// Package royalrush holds the Royal Rush behaviors of the Four Sepulchers:
// RoyalRushDefaultNpc, whose NPCs leave the world at minute 54 of the
// hour, and the classes derived from it: the afflicting watchmen of the
// graves, Halisha's treasure box, the room charms, the key boxes and the
// trigger boxes that start the rooms.
package royalrush

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the page directory of the Royal Rush behaviors.
const dir = "ai/individual/RoyalRushDefaultNpc"

// DefaultNpc is the RoyalRushDefaultNpc behavior. It binds no NPC of its
// own.
func DefaultNpc() script.Script {
	return script.Script{Dir: dir, Behavior: true, Hooks: DefaultNpcHooks()}
}

// DefaultNpcHooks are RoyalRushDefaultNpc's hooks, which its children
// derive theirs from: every second the NPC checks the local clock and
// deletes itself once it reads minute 54.
func DefaultNpcHooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3000", e.NPC, nil, 1000*time.Millisecond, 1000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3000") {
				if s.Now().Minute() == 54 {
					e.NPC.DeleteMe()
				}
			}
			return ""
		},
	}).Named("RoyalRushDefaultNpc")
}
