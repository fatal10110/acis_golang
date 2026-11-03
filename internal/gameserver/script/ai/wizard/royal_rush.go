package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const wizardDir = "ai/individual/Monster/WizardBase/Wizard"

// spellOrGiveUp makes n want to cast at castAt, weight 1000000, the skill
// its template lists under t, when it has the MP and HP the cast at
// checkOn takes; unchecked, the desire skips the reuse and cost checks.
// Otherwise n gives up its spells (int slot 0 set to 1) and wants to
// attack foe, weight 1000. A template that lists no such skill attacks.
func spellOrGiveUp(n *script.NPC, t npc.SkillType, checkOn, castAt, foe script.Creature, unchecked bool) {
	ref, _ := n.SkillByType(t)
	if !n.MeetsHPMPConditions(checkOn, ref) {
		n.SetInt(script.SlotIntAI0, 1)
		n.AddAttackDesire(foe, 1000)
		return
	}
	if unchecked {
		n.AddCastDesireUnchecked(castAt, ref, 1000000)
	} else {
		n.AddCastDesire(castAt, ref, 1000000)
	}
}

// RoyalRushHealer1 is a Wizard of the royal rush rooms that, until it gives
// up its spells, answers a playable's hit with its range debuff on the
// attacker on a draw under 33, else its self range DD magic, and casts the
// self range DD magic again after each cast while it hates someone. Any
// hit on its clan member may bring the range debuff down on the attacker,
// on a draw under 33. Short of MP, it attacks instead.
func RoyalRushHealer1() script.Script {
	base := WizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{18132, 18185, 18220}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				if s.Rnd(100) < 33 {
					castOrAttack(n, attacker, npc.SkillTypeWRangeDebuff, true)
				} else {
					spellOrGiveUp(n, npc.SkillTypeWSelfRangeDDMagic, n, n, attacker, true)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if s.Rnd(100) < 33 {
				castOrAttack(e.Called, e.Attacker, npc.SkillTypeWRangeDebuff, true)
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.Int(script.SlotIntAI0) != 1 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					spellOrGiveUp(n, npc.SkillTypeWSelfRangeDDMagic, n, n, mostHatedHI, true)
				}
			}
		},
	})}
}

// RoyalRushHealer2 is a Wizard of the royal rush rooms that, until it gives
// up its spells, may answer a playable's hit with its self range debuff on
// a draw under 33, then, on another draw under 33 against an attacker
// farther than 100, with its long range DD magic on it, else its self
// range DD magic. After each cast it strikes its most hated the same way.
// Any hit on its clan member may bring its range debuff down on the
// attacker, on a draw under 33. Short of MP, it attacks instead.
func RoyalRushHealer2() script.Script {
	base := WizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{18186, 18221}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				if s.Rnd(100) < 33 {
					spellOrGiveUp(n, npc.SkillTypeWSelfRangeDebuff, n, n, attacker, true)
				}
				if s.Rnd(100) < 33 && n.Distance2D(attacker) > 100 {
					castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic, true)
				} else {
					spellOrGiveUp(n, npc.SkillTypeWSelfRangeDDMagic, n, n, attacker, true)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if s.Rnd(100) < 33 {
				castOrAttack(e.Called, e.Attacker, npc.SkillTypeWRangeDebuff, true)
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil || n.Int(script.SlotIntAI0) == 1 {
				return
			}
			if s.Rnd(100) < 33 && n.Distance2D(mostHatedHI) > 100 {
				// The cost is checked on the NPC itself, and a shortfall
				// changes nothing.
				if wLongRangeDDMagic, _ := n.SkillByType(npc.SkillTypeWLongRangeDDMagic); n.MeetsHPMPConditions(n, wLongRangeDDMagic) {
					n.AddCastDesireUnchecked(mostHatedHI, wLongRangeDDMagic, 1000000)
				}
			} else {
				spellOrGiveUp(n, npc.SkillTypeWSelfRangeDDMagic, n, n, mostHatedHI, true)
			}
		},
	})}
}

// RoyalRushDDMagic2Hold is RoyalRushWizardDDMagic2Hold: a Wizard of the royal rush rooms that
// holds its spawn point, weight 30, when it has nothing to do. Until it
// gives up its spells, a playable's hit draws its long range DD magic on
// an attacker farther than 100 on a draw under 80, else its short range
// one, either only while it hates someone or on a draw under 2. A settled
// one with an empty hate list answers a playable's hit on its clan member
// the same way, by the attacker's distance from that member, and it
// strikes its most hated by distance after each cast. Once it gave up its
// spells, a playable's hit makes it want to attack by the damage.
func RoyalRushDDMagic2Hold() script.Script {
	base := WizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{18137, 18170, 18191, 18226}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
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
			isNullHate := n.HateList().Empty()
			if n.Distance2D(attacker) > 100 && s.Rnd(100) < 80 {
				if !isNullHate || s.Rnd(100) < 2 {
					castByType(n, attacker, npc.SkillTypeWLongRangeDDMagic, 1000000)
				}
			} else if !isNullHate || s.Rnd(100) < 2 {
				castByType(n, attacker, npc.SkillTypeWShortRangeDDMagic, 1000000)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				if caller.Distance2D(attacker) > 100 {
					castByType(called, attacker, npc.SkillTypeWLongRangeDDMagic, 1000000)
				} else {
					castByType(called, attacker, npc.SkillTypeWShortRangeDDMagic, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			mostHatedHI := n.HateList().MostHated()
			if mostHatedHI == nil || n.Int(script.SlotIntAI0) == 1 {
				return
			}
			if n.Distance2D(mostHatedHI) > 100 {
				castByType(n, mostHatedHI, npc.SkillTypeWLongRangeDDMagic, 1000000)
			} else {
				castByType(n, mostHatedHI, npc.SkillTypeWShortRangeDDMagic, 1000000)
			}
		},
	})}
}

// RoyalRushShortSelfRange is RoyalRushWizardShortSelfRange: a Wizard of the royal rush rooms that,
// until it gives up its spells, answers a playable's hit while it hates
// someone with its self range DD magic on a draw under 33, else its short
// range DD magic on the attacker; with an empty hate list only the short
// range one, on a draw under 2. A settled one with an empty hate list
// answers a playable's hit on its clan member with the short range one,
// and it casts it again at its most hated after each cast. Short of MP, it
// attacks instead; once it gave up its spells, a playable's hit makes it
// want to attack by the damage.
func RoyalRushShortSelfRange() script.Script {
	base := WizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{18140, 18172, 18194, 18229}, Hooks: base.With(script.Hooks{
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
			switch {
			case !n.HateList().Empty():
				if s.Rnd(100) < 33 {
					// The cost is checked against the attacker, the cast
					// aimed at the NPC itself.
					spellOrGiveUp(n, npc.SkillTypeWSelfRangeDDMagic, attacker, n, attacker, false)
				} else {
					castOrAttack(n, attacker, npc.SkillTypeWShortRangeDDMagic, false)
				}
			case s.Rnd(100) < 2:
				castOrAttack(n, attacker, npc.SkillTypeWShortRangeDDMagic, false)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				castOrAttack(called, attacker, npc.SkillTypeWShortRangeDDMagic, false)
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.Int(script.SlotIntAI0) != 1 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					castOrAttack(n, mostHatedHI, npc.SkillTypeWShortRangeDDMagic, false)
				}
			}
		},
	})}
}
