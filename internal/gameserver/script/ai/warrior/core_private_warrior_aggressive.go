package warrior

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// CorePrivateWarriorAggressive is CorePrivateWarriorAggressive: Core's
// Death Knight and Validus, WarriorAggressive guards that curse an
// attacker, and mute a caster, more than 8 levels above them, and every
// 90 to 330 seconds most likely shuffle whom they attack.
func CorePrivateWarriorAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{29007, 29010}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("3001", e.NPC, nil, coreShuffleDelay(s))
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				if s.Rnd(100) < 66 {
					e.NPC.RandomizeAttack()
				}
				s.StartTimer("3001", e.NPC, nil, coreShuffleDelay(s))
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			raidCurse(e.NPC, e.Attacker)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if raidMute(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
