package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// CorePrivatePremo is CorePrivatePremo: Core's Premo, a Warrior that curses
// an attacker, and mutes a caster, more than 8 levels above it.
func CorePrivatePremo() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{29013}, Hooks: base.With(script.Hooks{
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
