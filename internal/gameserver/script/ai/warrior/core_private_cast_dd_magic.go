package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const corePrivateCastDDMagicDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastDDMagic/CorePrivateWarriorCastDDMagicAggressive"

// CorePrivateCastDDMagicAggressive is
// CorePrivateWarriorCastDDMagicAggressive: a Core private, a
// WarriorCastDDMagicAggressive that curses an attacker, and mutes a
// caster, more than 8 levels above it. A hit adds no hate.
func CorePrivateCastDDMagicAggressive() script.Script {
	return script.Script{Dir: corePrivateCastDDMagicDir, Behavior: true, NPCs: []int32{29008, 29011}, Hooks: CorePrivateCastDDMagicAggressiveHooks()}
}

// CorePrivateCastDDMagicAggressiveHooks are
// CorePrivateWarriorCastDDMagicAggressive's hooks, which its children
// derive theirs from.
func CorePrivateCastDDMagicAggressiveHooks() script.Hooks {
	base := CastDDMagicAggressiveHooks()
	return base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if e.Caster.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}
			base.SeeSpell(s, e)
		},
	}).Named("CorePrivateWarriorCastDDMagicAggressive")
}

// CorePrivatePerum is CorePrivatePerum: Core's Perum, which every 90 to
// 330 s may shuffle its hate.
func CorePrivatePerum() script.Script {
	base := CorePrivateCastDDMagicAggressiveHooks()
	return script.Script{Dir: corePrivateCastDDMagicDir, Behavior: true, NPCs: []int32{29012}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("4001", e.NPC, nil, coreShuffleDelay(s))
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "4001") {
				if s.Rnd(100) < 66 {
					e.NPC.RandomizeAttack()
				}
				s.StartTimer("4001", e.NPC, nil, coreShuffleDelay(s))
			}
			return base.Timer(s, e)
		},
	})}
}

// coreShuffleDelay draws the time to a Core private's next hate shuffle:
// 90 s and up to 240 s more.
func coreShuffleDelay(s *script.Script) time.Duration {
	return time.Duration(90000+s.Rnd(240000)) * time.Millisecond
}
