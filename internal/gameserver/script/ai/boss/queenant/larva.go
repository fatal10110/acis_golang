package queenant

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Larva is the Queen Ant's larva. Every 5 seconds it leaves the world, its
// timers cancelled, once it has no live master. A hit from a player more
// than 8 levels above it wants its curse on that player; a spell it sees
// from such a player wants its silence on the caster.
func Larva() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/queenant", Behavior: true, NPCs: []int32{29002}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("2001", e.NPC, nil, 5000*time.Millisecond, 5000*time.Millisecond)

			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") {
				if !n.HasMaster() || n.Master().Dead() {
					n.DeleteMe()
					s.CancelTimers(script.TimerNPC(n))
				}
			}

			return base.Timer(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if caster.Level() > n.Level()+8 {
				n.AddCastDesire(caster, skill.Ref{ID: 4215, Level: 1}, 1000000)

				return
			}

			base.SeeSpell(s, e)
		},
	})}
}
