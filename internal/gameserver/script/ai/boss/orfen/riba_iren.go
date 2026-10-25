package orfen

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// RibaIren is Riba Iren, the healer of Orfen's clan. With nothing to do it
// wanders. A hit, or a spell it sees, from a player more than 8 levels
// above it makes it want a skill on that player; under half its HP a hit
// makes it want to heal itself. A clan member under half its HP that is
// hit makes it want to heal that member, Orfen nine times in ten, any
// other member one time in ten; another Riba Iren's call is ignored.
func RibaIren() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/orfen", Behavior: true, NPCs: []int32{29018}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}

			if n.HPRatio() < 0.5 {
				n.AddCastDesire(n, skill.Ref{ID: 4516, Level: 1}, 1000000)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			if caller.NpcID() == 29018 {
				return
			}

			chance := 10
			if caller.NpcID() == 29014 {
				chance = 90
			}
			if caller.HPRatio() < 0.5 && s.Rnd(100) < chance {
				called.AddCastDesire(caller, skill.Ref{ID: 4516, Level: 1}, 1000000)
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
