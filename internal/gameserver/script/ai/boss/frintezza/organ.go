package frintezza

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// OrganDummy (29052), the organ Frintezza plays, standing where he wakes.
// Appearing while Frintezza (global memo key 4) is at db value 4 or more it
// starts its guard a second later; otherwise it films his awakening at
// once: BS04_A, then twenty camera shots over about 76 s, wanting to attack
// Frintezza, weight 200, at the tenth. Its guard then repeats every 10 s:
// within 20 of Frintezza it wants to attack him, weight 200, else it leaves
// the world. Arg1 50000 of any script event films his last song a second
// later (six shots), arg1 8 his death 5 s later: two shots, then it drops
// every desire, wants to cast skill 5005 level 1 on Frintezza, weight
// 1000000, and films two more. Frintezza's death kills whoever of his
// party is told.
func OrganDummy() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29052}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			if c0 := memoNPC(s, "4"); c0 != nil && c0.DBValue() >= 4 {
				s.StartTimer("1020", e.NPC, nil, 1000*time.Millisecond)
				return
			}
			s.StartTimer("1000", e.NPC, nil, 1*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1000"):
				n.PlayMusic("BS04_A")
				n.SpecialCamera(script.CameraShot{Distance: 2500, Yaw: 180, Pitch: 20, Time: 0, Duration: 10000, Turn: 20, Rise: 90, Widescreen: 1, Unknown: 1})
				s.StartTimer("1001", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "1001"):
				n.SpecialCamera(script.CameraShot{Distance: 2500, Yaw: 180, Pitch: 20, Time: 100, Duration: 10000, Turn: 20, Rise: 90, Widescreen: 1, Unknown: 1})
				s.StartTimer("1002", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "1002"):
				n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 190, Pitch: 0, Time: 4000, Duration: 10000, Turn: 0, Rise: 30, Widescreen: 1, Unknown: 1})
				s.StartTimer("1003", n, nil, 4000*time.Millisecond)
			case strings.EqualFold(e.Name, "1003"):
				n.SpecialCamera(script.CameraShot{Distance: 100, Yaw: 180, Pitch: 0, Time: 5000, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1004", n, nil, 7300*time.Millisecond)
			case strings.EqualFold(e.Name, "1004"):
				n.SpecialCamera(script.CameraShot{Distance: 5, Yaw: 165, Pitch: -15, Time: 5000, Duration: 10000, Turn: 6, Rise: 15, Widescreen: 1, Unknown: 1})
				s.StartTimer("1005", n, nil, 7300*time.Millisecond)
			case strings.EqualFold(e.Name, "1005"):
				n.SpecialCamera(script.CameraShot{Distance: 1100, Yaw: 240, Pitch: 0, Time: 0, Duration: 10000, Turn: 210, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1006", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "1006"):
				n.SpecialCamera(script.CameraShot{Distance: 1100, Yaw: 240, Pitch: 0, Time: 0, Duration: 10000, Turn: 210, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1007", n, nil, 3400*time.Millisecond)
			case strings.EqualFold(e.Name, "1007"):
				n.SpecialCamera(script.CameraShot{Distance: 150, Yaw: 180, Pitch: 0, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1008", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "1008"):
				n.SpecialCamera(script.CameraShot{Distance: 150, Yaw: 180, Pitch: 0, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1009", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "1009"):
				n.SpecialCamera(script.CameraShot{Distance: 50, Yaw: 180, Pitch: 20, Time: 4000, Duration: 10000, Turn: 0, Rise: -5, Widescreen: 1, Unknown: 1})
				if c0 := memoNPC(s, "4"); c0 != nil {
					n.AddAttackDesire(c0, 200)
				}
				s.StartTimer("1010", n, nil, 5000*time.Millisecond)
			case strings.EqualFold(e.Name, "1010"):
				n.SpecialCamera(script.CameraShot{Distance: 0, Yaw: 280, Pitch: 40, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1011", n, nil, 2500*time.Millisecond)
			case strings.EqualFold(e.Name, "1011"):
				n.SpecialCamera(script.CameraShot{Distance: 500, Yaw: 230, Pitch: 45, Time: 7500, Duration: 10000, Turn: 15, Rise: 3, Widescreen: 1, Unknown: 1})
				s.StartTimer("1012", n, nil, 7500*time.Millisecond)
			case strings.EqualFold(e.Name, "1012"):
				n.SpecialCamera(script.CameraShot{Distance: 2100, Yaw: 200, Pitch: 13, Time: 9000, Duration: 10000, Turn: 4, Rise: 10, Widescreen: 1, Unknown: 1})
				s.StartTimer("1013", n, nil, 9500*time.Millisecond)
			case strings.EqualFold(e.Name, "1013"):
				n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 190, Pitch: 0, Time: 0, Duration: 10000, Turn: -50, Rise: 45, Widescreen: 1, Unknown: 1})
				s.StartTimer("1014", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "1014"):
				n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 190, Pitch: 0, Time: 0, Duration: 10000, Turn: -50, Rise: 45, Widescreen: 1, Unknown: 1})
				s.StartTimer("1015", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "1015"):
				n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 195, Pitch: 0, Time: 1800, Duration: 10000, Turn: -47, Rise: 42, Widescreen: 1, Unknown: 1})
				s.StartTimer("1016", n, nil, 1800*time.Millisecond)
			case strings.EqualFold(e.Name, "1016"):
				n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 195, Pitch: 0, Time: 600, Duration: 10000, Turn: -47, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("1017", n, nil, 2800*time.Millisecond)
			case strings.EqualFold(e.Name, "1017"):
				n.SpecialCamera(script.CameraShot{Distance: 1800, Yaw: 175, Pitch: 0, Time: 0, Duration: 10000, Turn: 40, Rise: -3, Widescreen: 1, Unknown: 1})
				s.StartTimer("1018", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "1018"):
				n.SpecialCamera(script.CameraShot{Distance: 1800, Yaw: 175, Pitch: 0, Time: 0, Duration: 10000, Turn: 40, Rise: -3, Widescreen: 1, Unknown: 1})
				s.StartTimer("1019", n, nil, 900*time.Millisecond)
			case strings.EqualFold(e.Name, "1019"):
				n.SpecialCamera(script.CameraShot{Distance: 2000, Yaw: 180, Pitch: 1, Time: 2000, Duration: 3000, Turn: 0, Rise: -5, Widescreen: 1, Unknown: 1})
				s.StartTimer("1020", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(e.Name, "1020"):
				if c0 := memoNPC(s, "4"); c0 != nil && n.Distance2D(c0) < 20 {
					n.AddAttackDesire(c0, 200)
				} else {
					n.DeleteMe()
				}
				s.StartTimer("1020", n, nil, 10000*time.Millisecond)
			case strings.EqualFold(e.Name, "2000"):
				n.SpecialCamera(script.CameraShot{Distance: 500, Yaw: 210, Pitch: 10, Time: 0, Duration: 10000, Turn: 10, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2001", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2001"):
				n.SpecialCamera(script.CameraShot{Distance: 500, Yaw: 210, Pitch: 10, Time: 0, Duration: 10000, Turn: 10, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2002", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "2002"):
				n.SpecialCamera(script.CameraShot{Distance: 5, Yaw: 210, Pitch: 10, Time: 5500, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2003", n, nil, 5500*time.Millisecond)
			case strings.EqualFold(e.Name, "2003"):
				n.SpecialCamera(script.CameraShot{Distance: 5, Yaw: 150, Pitch: 10, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2004", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "2004"):
				n.SpecialCamera(script.CameraShot{Distance: 5, Yaw: 150, Pitch: 10, Time: 0, Duration: 10000, Turn: 0, Rise: 0, Widescreen: 1, Unknown: 1})
				s.StartTimer("2005", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "2005"):
				n.SpecialCamera(script.CameraShot{Distance: 1500, Yaw: 180, Pitch: 10, Time: 6000, Duration: 6000, Turn: 0, Rise: 13, Widescreen: 1, Unknown: 1})
				s.StartTimer("2006", n, nil, 6000*time.Millisecond)
			case strings.EqualFold(e.Name, "3000"):
				n.SpecialCamera(script.CameraShot{Distance: 0, Yaw: 230, Pitch: 10, Time: 0, Duration: 10000, Turn: 0, Rise: -5, Widescreen: 1, Unknown: 1})
				s.StartTimer("3001", n, nil, 100*time.Millisecond)
			case strings.EqualFold(e.Name, "3001"):
				n.RemoveAllDesire()
				if c0 := memoNPC(s, "4"); c0 != nil {
					n.AddCastDesire(c0, skill.Ref{ID: 5005, Level: 1}, 1000000)
				}
				n.SpecialCamera(script.CameraShot{Distance: 0, Yaw: 230, Pitch: 10, Time: 0, Duration: 10000, Turn: 0, Rise: -5, Widescreen: 1, Unknown: 1})
				s.StartTimer("3002", n, nil, 400*time.Millisecond)
			case strings.EqualFold(e.Name, "3002"):
				n.SpecialCamera(script.CameraShot{Distance: 0, Yaw: 180, Pitch: 10, Time: 5000, Duration: 10000, Turn: 0, Rise: -5, Widescreen: 1, Unknown: 1})
				s.StartTimer("3006", n, nil, 7000*time.Millisecond)
			case strings.EqualFold(e.Name, "3006"):
				n.SpecialCamera(script.CameraShot{Distance: 1000, Yaw: 180, Pitch: 15, Time: 8000, Duration: 8000, Turn: 0, Rise: 10, Widescreen: 1, Unknown: 1})
				s.StartTimer("3007", n, nil, 8000*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if e.Caller.NpcID() == 29045 {
				e.Called.DoDie(e.Called)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.Arg1 == 50000 {
				s.StartTimer("2000", e.NPC, nil, 1000*time.Millisecond)
			} else if e.Arg1 == 8 {
				s.StartTimer("3000", e.NPC, nil, 5000*time.Millisecond)
			}
		},
	})}
}
