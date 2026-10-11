// Package guard holds the town guard behaviors: Guard, which attacks
// whoever hits it and the karma players it sees, plays its social
// animations and goes back to its spawn point when it strays out of its
// territory, and the GuardStand and GuardMoveAround behaviors derived from
// it.
package guard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the page directory of the guard behaviors.
const dir = "ai/individual/Guard"

// Guard is the Guard behavior. It binds no NPC of its own.
func Guard() script.Script {
	return script.Script{Dir: dir, Behavior: true, Hooks: GuardHooks()}
}

// GuardHooks are Guard's hooks, which its children derive theirs from.
func GuardHooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			moveAroundSocial1 := n.IntParam("MoveAroundSocial1")
			moveAroundSocial := n.IntParam("MoveAroundSocial")
			if moveAroundSocial > 0 || moveAroundSocial1 > 0 {
				s.StartTimerAtFixedRate("1671", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			}
			s.StartTimerAtFixedRate("9903", n, nil, 60000*time.Millisecond, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			e.NPC.AddAttackDesire(e.Attacker, 2000)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if _, ok := e.Creature.(*script.Player); ok && script.ActingPlayer(e.Creature).Karma() > 0 {
				e.NPC.AddAttackDesire(e.Creature, 1500)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1671") {
				if n.HPRatio() > 0.4 && !n.Dead() && n.Intention() != script.IntentionAttack {
					moveAroundSocial1 := n.IntParam("MoveAroundSocial1")
					moveAroundSocial := n.IntParam("MoveAroundSocial")
					if moveAroundSocial > 0 || moveAroundSocial1 > 0 {
						if moveAroundSocial > 0 && s.Rnd(100) < 40 {
							n.AddSocialDesire(3, int(moveAroundSocial*1000/30), 50)
						} else if moveAroundSocial1 > 0 && s.Rnd(100) < 40 {
							n.AddSocialDesire(3, int(moveAroundSocial1*1000/30), 50)
						}
					}
				}
			}
			if strings.EqualFold(e.Name, "9903") {
				if !n.InMyTerritory() {
					n.TeleportTo(n.SpawnLocation())
				}
			}
			return base.Timer(s, e)
		},
	}).Named("Guard")
}
