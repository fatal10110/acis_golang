package individual

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// SacrificialVictim is the Ritual Offering of the altar. Told of the
// altar's next stage (script event 3), it waits 10 s, plays its social
// action 1 and, 5 s later, leaves the Ritual Sacrifice 22145 where it
// stood as its private and leaves the world.
func SacrificialVictim() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{32038}, Hooks: base.With(script.Hooks{
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == 3 {
				s.StartTimer("998", e.NPC, nil, 10000*time.Millisecond)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "998"):
				n.AddSocialDesire(1, 7000, 1000)
				s.StartTimer("999", n, e.Player, 5000*time.Millisecond)
			case strings.EqualFold(e.Name, "999"):
				s.Spawner().CreateOnePrivateEx(n, 22145, here(n), 0, script.Params{})
				n.DeleteMe()
			}
			return base.Timer(s, e)
		},
	})}
}
