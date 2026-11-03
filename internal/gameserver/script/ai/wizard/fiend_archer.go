package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const fiendArcherDir = "ai/individual/Monster/WizardBase/Wizard/WizardFiendArcher"

// FiendArcher is WizardFiendArcher: a Wizard that hates a playable hitting
// it by the damage, 300 more when it hated no one, and shoots its fiend
// archer skill at it, unchecked. A settled one with an empty hate list
// answers a playable's hit on its clan member the same way, and it shoots
// its most hated after each cast. Short of MP it gives up its spells and
// attacks instead. It binds no NPC of its own.
func FiendArcher() script.Script {
	return script.Script{Dir: fiendArcherDir, Behavior: true, Hooks: fiendArcherHooks()}
}

// fiendArcherHooks are FiendArcher's hooks, which its children derive
// theirs from.
func fiendArcherHooks() script.Hooks {
	base := WizardHooks()
	return base.With(script.Hooks{
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
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
				castOrAttack(n, mostHatedHI, npc.SkillTypeWFiendArcher, true)
			}
			base.UseSkillFinished(s, e)
		},
	}).Named("WizardFiendArcher")
}

// FiendArcherAggressive is WizardFiendArcherAggressive: a
// WizardFiendArcher that, settled in its territory with an empty hate
// list, shoots its fiend archer skill, unchecked, at a playable it sees
// and hates it; short of MP it gives up its spells and attacks instead.
func FiendArcherAggressive() script.Script {
	base := fiendArcherHooks()
	return script.Script{Dir: fiendArcherDir, Behavior: true, NPCs: []int32{20819, 20841, 21617, 21618, 21085, 21619}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && hateList.Empty() && n.InMyTerritory() {
				castOrAttack(n, c, npc.SkillTypeWFiendArcher, true)
				hateList.AddDefault(c)
			}
			base.SeeCreature(s, e)
		},
	})}
}

// FiendArcherSelfBuff is WizardFiendArcherSelfBuff: a WizardFiendArcher
// that wants its self buff on itself as it appears and, hit by a playable
// while it neither attacks nor casts, again when it holds none of the
// buff's effect. It binds no NPC of its own.
func FiendArcherSelfBuff() script.Script {
	return script.Script{Dir: fiendArcherDir, Behavior: true, Hooks: fiendArcherSelfBuffHooks()}
}

// fiendArcherSelfBuffHooks are FiendArcherSelfBuff's hooks, which its
// child derives its from.
func fiendArcherSelfBuffHooks() script.Hooks {
	base := fiendArcherHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			castByType(e.NPC, e.NPC, npc.SkillTypeSelfBuff, 1000000)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Playable() && n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast {
				if selfBuff, ok := n.SkillByType(npc.SkillTypeSelfBuff); ok && s.AbnormalLevel(n, selfBuff) <= 0 {
					n.AddCastDesire(n, selfBuff, 1000000)
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WizardFiendArcherSelfBuff")
}

// FiendArcherSelfBuffAggressive is WizardFiendArcherSelfBuffAggressive: a
// WizardFiendArcherSelfBuff that, seeing any creature while it neither
// attacks nor casts, wants its self buff on itself when the creature seen
// holds none of the buff's effect, and wants to attack a playable it sees
// in its territory.
func FiendArcherSelfBuffAggressive() script.Script {
	base := fiendArcherSelfBuffHooks()
	return script.Script{Dir: fiendArcherDir, Behavior: true, NPCs: []int32{21388}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast {
				if selfBuff, ok := n.SkillByType(npc.SkillTypeSelfBuff); ok && s.AbnormalLevel(c, selfBuff) <= 0 {
					n.AddCastDesire(n, selfBuff, 1000000)
				}
			}
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}
