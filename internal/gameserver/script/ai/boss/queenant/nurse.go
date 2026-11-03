package queenant

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Nurse is the Queen Ant's nurse ant. Every 5 seconds it leaves the world,
// its timers cancelled, once it has no live master. With nothing to do it
// follows its live master, or wanders. A hit from a player more than 8
// levels above it wants its curse on that player and drops its hate of it;
// a spell it sees from such a player wants its silence on the caster. A
// hit on the Queen Ant wants her heal on her, unless the nurse, over 2500
// away from her, is casting on the larva; a hit on the larva wants both
// heals on it, lightly.
func Nurse() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/queenant", Behavior: true, NPCs: []int32{29003}, Hooks: base.With(script.Hooks{
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
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.HasMaster() && !n.Master().Dead() {
				n.AddFollowDesire(n.Master(), 20)
			} else {
				n.AddWanderDesire(40, 20)
			}
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)

				n.StopHate(attacker)
			}
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			caller, called := e.Caller, e.Called
			if caller.NpcID() == 29001 {
				top := called.TopDesireTarget()
				if called.Distance2D(caller) > 2500 && called.Intention() == script.IntentionCast {
					if larva, ok := top.(*script.NPC); ok && larva.NpcID() == 29002 {
						return
					}
				}

				called.AddCastDesire(caller, skill.Ref{ID: 4020, Level: 1}, 1000000)
			} else if caller.NpcID() == 29002 {
				called.AddCastDesire(caller, skill.Ref{ID: 4020, Level: 1}, 100)

				called.AddCastDesire(caller, skill.Ref{ID: 4024, Level: 1}, 100)
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
