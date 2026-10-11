// Package monster holds MonsterAI, the behavior every monster behavior
// derives from.
package monster

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// New is MonsterAI. It binds no NPC of its own.
func New() script.Script {
	return script.Script{Dir: "ai", Behavior: true, Hooks: Hooks()}
}

// Hooks are MonsterAI's hooks: once a monster's attack kills a summon, it
// turns on the summon's owner.
func Hooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnAttackFinished: func(_ *script.Script, e script.AttackFinished) {
			if e.Target != nil && e.Target.Summon() && e.Target.Dead() {
				if p := script.ActingPlayer(e.Target); p != nil {
					e.NPC.AddAttackDesire(p, 500)
				}
			}
		},
	})
}
