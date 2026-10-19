package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderDD2 is PartyLeaderWizardDD2: a PartyLeaderWizard that strikes
// what it fights with its long range DD magic from farther than 100, else
// with its short range one, and attacks instead once it cannot pay for a
// spell. It binds no NPC of its own.
func PartyLeaderDD2() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, Hooks: partyLeaderDD2Hooks()}
}

// dd2Spell makes n answer c with the DD magic its range from c calls for.
func dd2Spell(n *script.NPC, c script.Creature) {
	if n.Distance2D(c) > 100 {
		castOrAttack(n, c, npc.SkillTypeWLongRangeDDMagic, true)
	} else {
		castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, true)
	}
}

// partyLeaderDD2Hooks are PartyLeaderWizardDD2's hooks, which its children
// derive theirs from.
func partyLeaderDD2Hooks() script.Hooks {
	base := partyLeaderHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			if !attacker.Playable() {
				return
			}
			if n.Int(script.SlotIntAI0) != 0 {
				f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				n.AddAttackDesire(attacker, f0*100)
				return
			}
			hated := n.HateList().MostHated() != nil
			if n.Distance2D(attacker) > 100 && s.Rnd(100) < 80 {
				if hated || s.Rnd(100) < 2 {
					castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic, true)
				}
			} else if hated || s.Rnd(100) < 2 {
				castOrAttack(n, attacker, npc.SkillTypeWShortRangeDDMagic, true)
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
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			hateList := called.HateList()
			hateList.Refresh()
			if target.Playable() && hateList.Empty() {
				dd2Spell(called, target)
			}
			base.PartyAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.Int(script.SlotIntAI0) != 1 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					dd2Spell(n, mostHatedHI)
				}
			}
		},
	}).Named("PartyLeaderWizardDD2")
}
