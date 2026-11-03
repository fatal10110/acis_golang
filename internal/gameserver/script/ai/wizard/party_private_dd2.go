package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PartyPrivateDD2 is PartyPrivateWizardDD2: a PartyPrivateWizard that
// strikes what it fights with its long range DD magic from farther than
// 100, else with its short range one, and attacks instead once it cannot
// pay for a spell. A hit on its party strikes the attacker once its hate
// list holds a creature; a hit on its clan strikes the attacker while it
// holds none.
func PartyPrivateDD2() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21542, 21545}, Hooks: partyPrivateDD2Hooks()}
}

// partyPrivateDD2Hooks are PartyPrivateWizardDD2's hooks, which its
// children derive theirs from.
func partyPrivateDD2Hooks() script.Hooks {
	base := partyPrivateHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			if e.Attacker.Playable() {
				dd2Attacked(s, e)
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			base.PartyAttacked(s, e)
			called, target := e.Called, e.Target
			hateList := called.HateList()
			hateList.Refresh()
			if target.Playable() && !hateList.Empty() {
				dd2Spell(called, target)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				dd2Spell(called, attacker)
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil && n.Int(script.SlotIntAI0) != 1 {
				dd2Spell(n, mostHatedHI)
			}
		},
	}).Named("PartyPrivateWizardDD2")
}
