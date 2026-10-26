package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const saintDDMagic2Dir = "ai/individual/Monster/WizardBase/Wizard/WizardSaintBasic/WizardSaintDDMagic2"

// saintNearOr makes n want, unchecked, its short range DD magic on target
// standing within 40 on a draw under 33, else the skill its template lists
// under far; short of MP it gives up its spells and attacks target instead.
func saintNearOr(s *script.Script, n *script.NPC, target script.Creature, far npc.SkillType) {
	if s.Rnd(100) < 33 && n.Distance2D(target) < 40 {
		castOrAttack(n, target, npc.SkillTypeWShortRangeDDMagic, true)
	} else {
		castOrAttack(n, target, far, true)
	}
}

// SaintDDMagic2 is WizardSaintDDMagic2: a WizardSaintBasic that, until it
// gives up its spells, answers a playable's hit from farther than 100,
// while it hates someone and on a draw under 80, with its long range DD
// magic, unchecked; once it gave up its spells it wants to attack the
// playable by the damage. After each cast, until it gives up its spells,
// it wants its debuff on its most hated holding none of its effect on a
// draw under 33, then its short range DD magic within 40 on a draw under
// 33, else its long range one, both unchecked; once it gave up its spells
// it wants to attack its most hated, weight 1000. Short of MP it gives up
// its spells and attacks instead.
func SaintDDMagic2() script.Script {
	return script.Script{Dir: saintDDMagic2Dir, Behavior: true, NPCs: []int32{21537, 21527}, Hooks: saintDDMagic2Hooks()}
}

// saintDDMagic2Hooks are SaintDDMagic2's hooks, which its child derives
// its own from.
func saintDDMagic2Hooks() script.Hooks {
	base := saintBasicHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			base.Attacked(s, e)
			if attacker.Playable() {
				if n.Int(script.SlotIntAI0) == 0 {
					if !n.HateList().Empty() && n.Distance2D(attacker) > 100 && s.Rnd(100) < 80 {
						castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic, true)
					}
				} else {
					f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
					n.AddAttackDesire(attacker, f0*100)
				}
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil {
				return
			}
			if n.Int(script.SlotIntAI0) == 0 {
				if saintDebuffs(s, n, mostHatedHI) {
					castOrAttack(n, mostHatedHI, npc.SkillTypeDebuff, false)
				}
				saintNearOr(s, n, mostHatedHI, npc.SkillTypeWLongRangeDDMagic)
			} else {
				n.AddAttackDesire(mostHatedHI, 1000)
			}
		},
	}).Named("WizardSaintDDMagic2")
}

// SaintRangeDDMagic2Aggressive is WizardSaintRangeDDMagic2Aggressive: a
// SaintDDMagic2 that, settled in its territory with an empty hate list,
// hates a playable it sees and wants its short range DD magic on it within
// 40, else its long range one, both unchecked. A hit from farther than 100
// by a party member, or by anyone it does not most want to act on, wants
// on a draw under 33 its second long range DD magic, unchecked, on the
// attacker. Short of MP it gives up its spells and attacks instead.
func SaintRangeDDMagic2Aggressive() script.Script {
	base := saintDDMagic2Hooks()
	return script.Script{Dir: saintDDMagic2Dir, Behavior: true, NPCs: []int32{21532, 21523}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			hateList := n.HateList()
			if n.LifeTime() > 7 && n.InMyTerritory() && hateList.Empty() {
				if n.Distance2D(c) < 40 {
					castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, true)
				} else {
					castOrAttack(n, c, npc.SkillTypeWLongRangeDDMagic, true)
				}
				hateList.AddDefault(c)
			}
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			player := script.ActingPlayer(attacker)
			inParty := player != nil && player.InParty()
			if (inParty || !script.Same(n.TopDesireTarget(), attacker)) && n.Distance2D(attacker) > 100 {
				if s.Rnd(100) < 33 {
					castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic2, true)
				}
			}
			base.Attacked(s, e)
		},
	})}
}
