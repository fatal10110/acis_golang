package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TimerBomb is WarriorTimerBomb: a bomb of the Frintezza raid that blows
// up around itself 10 to 58 seconds after it appears, or as soon as it is
// hit from closer than 200, and dies when that blast lands. Meanwhile it
// switches between walking and running every 3 to 8 seconds.
//
// Its memory: IntAI0 is set while it walks.
func TimerBomb() script.Script {
	base := WarriorHooks()
	blast := func(n *script.NPC) { addCastByType(n, n, npc.SkillTypeSelfRangeDDMagic, 1000000) }
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{18340, 18341}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			i0 := s.Rnd(59)
			if i0 < 10 {
				i0 += 10
			}
			s.StartTimer("1009", n, nil, time.Duration(i0)*time.Second)
			i1 := s.Rnd(9) + 1
			s.StartTimer("1010", n, nil, time.Duration(i1)*time.Second)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.NPC.Distance2D(e.Attacker) < 200 {
				blast(e.NPC)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1009") {
				blast(n)
			}
			if strings.EqualFold(e.Name, "1010") {
				if n.Int(script.SlotIntAI0) == 0 {
					n.SetRunning(false)
					n.SetInt(script.SlotIntAI0, 1)
				} else {
					n.SetRunning(true)
					n.SetInt(script.SlotIntAI0, 0)
				}
				i1 := s.Rnd(9)
				if i1 <= 2 {
					i1 += 3
				}
				s.StartTimer("1010", n, nil, time.Duration(i1)*time.Second)
			}
			return base.Timer(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if isSkillOfType(n, e.Skill, npc.SkillTypeSelfRangeDDMagic) && e.Success {
				n.DoDie(n)
			}
			base.UseSkillFinished(s, e)
		},
	})}
}
