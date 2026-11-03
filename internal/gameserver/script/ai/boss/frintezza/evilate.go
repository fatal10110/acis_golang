package frintezza

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// EvilateA (29049), a portrait of the tomb. 45 s after it appears, then
// every 20 s, it raises a portrait spirit (29050) as its private at the
// point and heading its SpawnPosX, SpawnPosY, SpawnPosZ and SpawnAngle AI
// parameters name, four in all. A hit with skill 2276 kills it. Told that
// one of its party died while it is a dead master, it asks the dead one's
// spawn to bring it back in 20 s.
func EvilateA() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29049}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("1000", e.NPC, nil, 45000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			at := spiritPoint(n)
			switch {
			case strings.EqualFold(e.Name, "1000"):
				s.Spawner().CreateOnePrivateEx(n, 29050, at, 0, script.Params{})
				s.StartTimer("1001", n, nil, 20000*time.Millisecond)
			case strings.EqualFold(e.Name, "1001"):
				s.Spawner().CreateOnePrivateEx(n, 29050, at, 0, script.Params{})
				s.StartTimer("1002", n, nil, 20000*time.Millisecond)
			case strings.EqualFold(e.Name, "1002"):
				s.Spawner().CreateOnePrivateEx(n, 29050, at, 0, script.Params{})
				s.StartTimer("1003", n, nil, 20000*time.Millisecond)
			case strings.EqualFold(e.Name, "1003"):
				s.Spawner().CreateOnePrivateEx(n, 29050, at, 0, script.Params{})
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Skill.ID == 2276 {
				e.NPC.DoDie(e.NPC)
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			if !script.Same(e.Called, e.Caller) && e.Called.IsMaster() && e.Called.Dead() {
				s.Spawner().ScheduleRespawn(e.Caller, 20000*time.Millisecond)
			}
		},
	})}
}

// EvilateB (29048) is EvilateA raising two winged portrait spirits
// (29051), 45 s and 65 s after it appears.
func EvilateB() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29048}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("1000", e.NPC, nil, 45000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			at := spiritPoint(n)
			switch {
			case strings.EqualFold(e.Name, "1000"):
				s.Spawner().CreateOnePrivateEx(n, 29051, at, 0, script.Params{})
				s.StartTimer("1001", n, nil, 20000*time.Millisecond)
			case strings.EqualFold(e.Name, "1001"):
				s.Spawner().CreateOnePrivateEx(n, 29051, at, 0, script.Params{})
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Skill.ID == 2276 {
				e.NPC.DoDie(e.NPC)
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			if !script.Same(e.Called, e.Caller) && e.Called.IsMaster() && e.Called.Dead() {
				s.Spawner().ScheduleRespawn(e.Caller, 20000*time.Millisecond)
			}
		},
	})}
}

// spiritPoint is where a portrait raises its spirits: the point and heading
// its SpawnPosX, SpawnPosY, SpawnPosZ and SpawnAngle AI parameters name.
func spiritPoint(n *script.NPC) script.Loc {
	return script.Loc{X: int(n.IntParam("SpawnPosX")), Y: int(n.IntParam("SpawnPosY")), Z: int(n.IntParam("SpawnPosZ")), Heading: int(n.IntParam("SpawnAngle"))}
}
