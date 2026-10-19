package individual

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// TriolCamera films the altar's sacrifice. Told of the altar's next stage
// (script event 3), it plays BS04_A, keeps the creature the event names in
// its first int slot and, a second later, plays five camera shots around
// itself, 300 ms, 300 ms, 9.4 s and 5 s apart; 6 s after the last it tells
// the NPCs within 4000 that the scene is over (event 4 naming that
// creature), when one was named.
func TriolCamera() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{13015}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == 3 {
				e.NPC.PlayMusic("BS04_A")
				e.NPC.SetInt(script.SlotIntAI0, e.Arg1)
				s.StartTimer("1001", e.NPC, nil, 1000*time.Millisecond)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1001"):
				n.SpecialCamera(script.CameraShot{Distance: 1500, Yaw: 88, Pitch: 89, Time: 0, Duration: 10000, Turn: 0, Rise: 110, Widescreen: 1, Unknown: 0})
				s.StartTimer("1002", n, nil, 300*time.Millisecond)
			case strings.EqualFold(e.Name, "1002"):
				n.SpecialCamera(script.CameraShot{Distance: 1500, Yaw: 88, Pitch: 89, Time: 0, Duration: 10000, Turn: 0, Rise: 110, Widescreen: 1, Unknown: 0})
				s.StartTimer("1003", n, nil, 300*time.Millisecond)
			case strings.EqualFold(e.Name, "1003"):
				n.SpecialCamera(script.CameraShot{Distance: 450, Yaw: 88, Pitch: 3, Time: 5500, Duration: 10000, Turn: 0, Rise: 350, Widescreen: 1, Unknown: 0})
				s.StartTimer("1004", n, nil, 9400*time.Millisecond)
			case strings.EqualFold(e.Name, "1004"):
				n.SpecialCamera(script.CameraShot{Distance: 500, Yaw: 88, Pitch: 4, Time: 5000, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 0})
				s.StartTimer("1005", n, nil, 5000*time.Millisecond)
			case strings.EqualFold(e.Name, "1005"):
				n.SpecialCamera(script.CameraShot{Distance: 3000, Yaw: 88, Pitch: 4, Time: 6000, Duration: 6900, Turn: 0, Rise: 15, Widescreen: 1, Unknown: 0})
				s.StartTimer("1006", n, nil, 6000*time.Millisecond)
			case strings.EqualFold(e.Name, "1006"):
				if n.Int(script.SlotIntAI0) != 0 {
					s.BroadcastScriptEvent(n, 4, n.Int(script.SlotIntAI0), 4000)
				}
			}
			return base.Timer(s, e)
		},
	})}
}
