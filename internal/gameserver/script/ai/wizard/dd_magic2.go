package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const ddMagic2Dir = "ai/individual/Monster/WizardBase/Wizard/WizardDDMagic2"

// ddMagic2Spell makes n want to cast at c, unchecked, its long range DD
// magic when c stands farther than 100, else its short range one; short
// of MP it gives up its spells and attacks c instead.
func ddMagic2Spell(n *script.NPC, c script.Creature) {
	if n.Distance2D(c) > 100 {
		castOrAttack(n, c, npc.SkillTypeWLongRangeDDMagic, true)
	} else {
		castOrAttack(n, c, npc.SkillTypeWShortRangeDDMagic, true)
	}
}

// DDMagic2 is WizardDDMagic2: a Wizard that, until it gives up its spells,
// answers a playable's hit with its long range DD magic on an attacker
// farther than 100 on a draw under 80, else its short range one, either
// only while it hates someone or on a draw under 2. A settled one with an
// empty hate list answers a playable's hit on its clan member by the
// attacker's distance, and it strikes its most hated by distance after
// each cast. Short of MP it attacks instead; once it gave up its spells, a
// playable's hit makes it want to attack by the damage.
func DDMagic2() script.Script {
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{
		20092, 20331, 20426, 20427, 20428, 20429, 21257, 21268, 21281, 21289, 21681, 21704, 21727, 21750, 21773, 21796, 21024, 20269,
		21277, 20197, 21110, 20265, 20059, 20055, 21640, 21592, 22021, 22036, 22044, 22047, 22081, 22082, 22034, 22042, 18147,
	}, Hooks: ddMagic2Hooks()}
}

// ddMagic2Hooks are DDMagic2's hooks, which its children derive theirs
// from.
func ddMagic2Hooks() script.Hooks {
	base := WizardHooks()
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
			i0 := n.HateList().MostHated() != nil
			if n.Distance2D(attacker) > 100 && s.Rnd(100) < 80 {
				if i0 || s.Rnd(100) < 2 {
					castOrAttack(n, attacker, npc.SkillTypeWLongRangeDDMagic, true)
				}
			} else if i0 || s.Rnd(100) < 2 {
				castOrAttack(n, attacker, npc.SkillTypeWShortRangeDDMagic, true)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				ddMagic2Spell(called, attacker)
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.Int(script.SlotIntAI0) != 1 {
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					ddMagic2Spell(n, mostHatedHI)
				}
			}
		},
	}).Named("WizardDDMagic2")
}

// RoyalRushClanBuff is RoyalRushWizardClanBuff: a WizardDDMagic2 of the
// royal rush rooms that, settled with an empty hate list, answers a
// playable's hit on its clan member with its clan buff, cast at the
// attacker; short of MP it gives up its spells and attacks instead.
func RoyalRushClanBuff() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{18138, 18145, 18171, 18192, 18227, 21404}, Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			called.HateList().Refresh()
			if called.LifeTime() > 7 && attacker.Playable() && called.HateList().Empty() {
				castOrAttack(called, attacker, npc.SkillTypeWClanBuff, false)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// RoyalRushRangeDebuff is RoyalRushWizardRangeDebuff: a WizardDDMagic2 of
// the royal rush rooms that, until it gives up its spells, may answer a
// playable's hit on its clan member with its range debuff on the attacker,
// on a draw under 33; short of MP it gives up its spells and attacks
// instead.
func RoyalRushRangeDebuff() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{18139, 18193, 18228}, Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 33 {
				castOrAttack(called, attacker, npc.SkillTypeWRangeDebuff, false)
			}
			base.ClanAttacked(s, e)
		},
	})}
}
