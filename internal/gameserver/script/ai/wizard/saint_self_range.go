package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const saintSelfRangeDir = "ai/individual/Monster/WizardBase/Wizard/WizardSaintBasic/WizardSaintSelfRangeDDMagic"

// SaintSelfRangeDDMagic is WizardSaintSelfRangeDDMagic: a WizardSaintBasic
// that answers a hit within 40 by a party member, or by anyone it does not
// most want to act on, on a draw under 33, with its self range DD magic on
// itself, unchecked; short of MP it gives up its spells and attacks the
// attacker. Until it gives up its spells, a playable's hit from farther
// than 100 on a draw under 80, while it hates someone or on a draw under
// 2, wants its middle range DD magic, unchecked, on the attacker; once it
// gave up its spells it wants to attack the playable by the damage. At the
// end of a walk it wants its debuff on its most hated holding none of its
// effect on a draw under 33, then its short range DD magic within 40 on a
// draw under 33, else its middle range one, unchecked. After each cast,
// until it gives up its spells, it runs from its most hated within 100,
// weight 1000000, else may debuff it the same way, then strikes it as at
// the end of a walk, both strikes unchecked; once it gave up its spells it
// wants to attack its most hated, weight 1000. Short of MP it gives up its
// spells and attacks instead.
func SaintSelfRangeDDMagic() script.Script {
	return script.Script{Dir: saintSelfRangeDir, Behavior: true, NPCs: []int32{21650}, Hooks: saintSelfRangeHooks()}
}

// saintSelfRangeHooks are SaintSelfRangeDDMagic's hooks, which its child
// derives its own from.
func saintSelfRangeHooks() script.Hooks {
	base := saintBasicHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			player := script.ActingPlayer(attacker)
			inParty := player != nil && player.InParty()
			if (inParty || !script.Same(attacker, n.TopDesireTarget())) && n.Distance2D(attacker) < 40 && s.Rnd(100) < 33 {
				// The cost is checked against itself, but the attack on a
				// shortfall goes to the attacker.
				ref, _ := n.SkillByType(npc.SkillTypeWSelfRangeDDMagic)
				if n.MeetsHPMPConditions(n, ref) {
					n.AddCastDesireUnchecked(n, ref, 1000000)
				} else {
					n.SetInt(script.SlotIntAI0, 1)
					n.AddAttackDesire(attacker, 1000)
				}
			}
			base.Attacked(s, e)
			if attacker.Playable() {
				if n.Int(script.SlotIntAI0) == 0 {
					if n.Distance2D(attacker) > 100 && s.Rnd(100) < 80 && (!n.HateList().Empty() || s.Rnd(100) < 2) {
						castOrAttack(n, attacker, npc.SkillTypeWMiddleRangeDDMagic, true)
					}
				} else {
					f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
					n.AddAttackDesire(attacker, f0*100)
				}
			}
			base.Attacked(s, e)
		},
		OnMoveToFinished: func(s *script.Script, e script.MoveToFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil {
				return
			}
			if saintDebuffs(s, n, mostHatedHI) {
				castOrAttack(n, mostHatedHI, npc.SkillTypeDebuff, false)
			}
			// Unlike every other strike of the family, this short range one
			// is checked.
			if s.Rnd(100) < 33 && n.Distance2D(mostHatedHI) < 40 {
				castOrAttack(n, mostHatedHI, npc.SkillTypeWShortRangeDDMagic, false)
			} else {
				castOrAttack(n, mostHatedHI, npc.SkillTypeWMiddleRangeDDMagic, true)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil {
				return
			}
			if n.Int(script.SlotIntAI0) == 0 {
				if n.Distance2D(mostHatedHI) < 100 {
					n.AddFleeDesire(mostHatedHI, n.MaxDriftRange(), 1000000)
				} else if saintDebuffs(s, n, mostHatedHI) {
					castOrAttack(n, mostHatedHI, npc.SkillTypeDebuff, false)
				}
				saintNearOr(s, n, mostHatedHI, npc.SkillTypeWMiddleRangeDDMagic)
			} else {
				n.AddAttackDesire(mostHatedHI, 1000)
			}
		},
	}).Named("WizardSaintSelfRangeDDMagic")
}

// SaintSelfRangeDDMagicAggressive is WizardSaintSelfRangeDDMagicAggressive:
// a SaintSelfRangeDDMagic that, settled in its territory with an empty hate
// list, hates a playable it sees and wants its short range DD magic on it
// within 40, else its middle range one, both unchecked; short of MP it
// gives up its spells and attacks instead.
func SaintSelfRangeDDMagicAggressive() script.Script {
	base := saintSelfRangeHooks()
	return script.Script{Dir: saintSelfRangeDir, Behavior: true, NPCs: []int32{21535, 21529}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && hateList.Empty() && n.InMyTerritory() {
				if n.Distance2D(c) < 40 {
					castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, true)
				} else {
					castOrAttack(n, c, npc.SkillTypeWMiddleRangeDDMagic, true)
				}
				hateList.AddDefault(c)
			}
			base.SeeCreature(s, e)
		},
	})}
}
