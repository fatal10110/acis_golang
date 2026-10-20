package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Medusa is Medusa: a WarriorAggressive that, below a fifth of its HP and
// hit by the creature it hates most, now and then calls out and wants to
// cast its fatal poison on it.
func Medusa() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{20158}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if s.Rnd(100) < 3 && n.HPRatio() < 0.2 && script.Same(n.MostHated(), e.Attacker) {
				n.Say(script.NpcString(1000452))
				n.AddCastDesire(e.Attacker, skill.Ref{ID: 4320, Level: 3}, 1000000)
			}
			base.Attacked(s, e)
		},
	})}
}
