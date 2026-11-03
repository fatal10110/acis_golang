package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyPrivateDD2Heal is PartyPrivateWizardDD2Heal: a PartyPrivateWizardDD2
// that may heal itself after a playable's hit below 70% of its HP, and its
// party member after a playable's hit on it below 70% of that member's HP,
// each on a draw under 33. A settled one with an empty hate list may heal
// its clan member a playable hits, on a draw under 33.
func PartyPrivateDD2Heal() script.Script {
	base := partyPrivateDD2Hooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21543, 21546, 21823}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n := e.NPC
			if e.Attacker.Playable() && s.Rnd(100) < 33 && n.HPRatio() < 0.7 {
				castByType(n, n, npc.SkillTypeMagicHeal, 1000000)
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			base.PartyAttacked(s, e)
			caller, called := e.Caller, e.Called
			if e.Target.Playable() && s.Rnd(100) < 33 && caller.HPRatio() < 0.7 {
				castByType(called, caller, npc.SkillTypeMagicHeal, 1000000)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.HateList().Empty() && s.Rnd(100) < 33 {
				castByType(called, caller, npc.SkillTypeMagicHeal, 1000000)
			}
			base.ClanAttacked(s, e)
		},
	})}
}
