package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// QueenAntPrivateGuardAnt is QueenAntPrivateGuardAnt: Queen Ant's guard
// ant, a WarriorAggressive that every 90 to 330 seconds most likely
// shuffles whom it attacks, every 10 seconds jumps home forgetting its
// targets when it strays out of its territory, and turns on a playable
// that attacks its clan by the damage dealt.
func QueenAntPrivateGuardAnt() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{29004}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("3001", e.NPC, nil, coreShuffleDelay(s))
			s.StartTimerAtFixedRate("3002", e.NPC, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "3001") {
				if s.Rnd(100) < 66 {
					n.RandomizeAttack()
				}
				s.StartTimer("3001", n, nil, coreShuffleDelay(s))
			} else if strings.EqualFold(e.Name, "3002") {
				if !n.InMyTerritory() {
					n.TeleportTo(n.SpawnLocation())
					n.RemoveAllAttackDesire()
				}
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				hateRatio := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
				called.AddAttackDesire(attacker, float64(int(hateRatio*50)))
			}
		},
	})}
}
