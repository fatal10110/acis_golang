package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Agit02VampireArcherAgit is Agit02VampireArcherAgit: a clan hall siege
// archer, a Wizard that never wanders. It shoots its fiend arrow, or
// attacks once it cannot pay for it, at every creature it sees, at a
// playable that hits it, at a playable that hits its clan while its hate
// list is empty, and at its most hated once a cast ends. A hit on its door
// makes it want to attack the attacker by 50 for each twentieth of its
// maximum hit points the damage makes; a siege summon's hit makes it want
// to attack the summon's owner, weight 5000, and the summon, weight 1000.
func Agit02VampireArcherAgit() script.Script {
	base := WizardHooks()
	return script.Script{Dir: "ai/individual/Monster/WizardBase/Wizard", Behavior: true, NPCs: []int32{35634}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			castOrAttack(e.NPC, e.Creature, npc.SkillTypeWFiendArcher, true)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				hateList := n.HateList()
				if hateList.Empty() {
					hateList.Add(attacker, (f0*100)+300)
				} else {
					hateList.Add(attacker, f0*100)
				}
				if !hateList.Empty() {
					castOrAttack(n, attacker, npc.SkillTypeWFiendArcher, true)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				castOrAttack(called, attacker, npc.SkillTypeWFiendArcher, true)
			}
			base.ClanAttacked(s, e)
		},
		OnStaticObjectClanAttacked: func(_ *script.Script, e script.StaticObjectClanAttacked) {
			called, attacker := e.Called, e.Attacker
			switch {
			case attacker.SiegeSummon():
				called.AddAttackDesire(script.ActingPlayer(attacker), 5000)
				called.AddAttackDesire(attacker, 1000)
			case attacker.Playable():
				called.AddAttackDesire(attacker, ((float64(e.Damage)*1.0)/float64(called.MaxHP()))/0.05*50)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
				castOrAttack(n, mostHatedHI, npc.SkillTypeWFiendArcher, true)
			}
			base.UseSkillFinished(s, e)
		},
	})}
}
