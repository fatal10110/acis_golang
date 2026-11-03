package wizard

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// CastClanBuffCurseStoneAggressive is
// WizardCastClanBuffCurseStoneAggressive: a Ketra or Varka WizardDDMagic2
// that petrifies a playable attacker allied with its own faction, and a
// player who helps one, and ignores such a player it sees, attacking any
// other. Until it gives up its spells a playable's hit, with its hate list
// filled, may buff it once above half of its HP and may debuff its most
// hated attacker; a settled one answers a playable's hit on its clan member
// the same way, its debuff aimed at itself.
func CastClanBuffCurseStoneAggressive() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21338, 21329, 21364, 21355}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if defaultnpc.MaybeCastPetrify(n, attacker) {
				return
			}
			base.Attacked(s, e)
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				hateList := n.HateList()
				if !hateList.Empty() {
					buffOnce(s, n, script.SlotIntAI2)
					if mostHated := hateList.MostHated(); mostHated != nil && script.Same(attacker, mostHated) {
						curseOrAttack(s, n, attacker, attacker, npc.SkillTypeDebuff)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if defaultnpc.MaybeCastPetrify(called, attacker) {
				return
			}
			if attacker.Playable() && called.LifeTime() > 7 {
				buffOnce(s, called, script.SlotIntAI2)
				curseOrAttack(s, called, attacker, called, npc.SkillTypeDebuff)
			}
			base.ClanAttacked(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI2, 0)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil || alliedWithFaction(n, p) {
				return
			}
			defaultnpc.TryToAttack(s, n, e.Creature)
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

// CastClanBuffRangeCurse is WizardCastClanBuffRangeCurse: a WizardDDMagic2
// that, on a playable's hit, may buff itself once above half of its HP
// and, when its top desire target is someone else, may cast its range
// debuff around itself; a settled one answers a playable's hit on its clan
// member the same way, whatever its top desire target.
func CastClanBuffRangeCurse() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21430}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI2, 0)
			e.NPC.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				buffOnce(s, n, script.SlotIntAI3)
				if top := n.TopDesireTarget(); top != nil && !script.Same(top, attacker) {
					curseOrAttack(s, n, attacker, n, npc.SkillTypeRangeDebuff)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				buffOnce(s, called, script.SlotIntAI3)
				curseOrAttack(s, called, attacker, called, npc.SkillTypeRangeDebuff)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// CastClanBuffRangeDDCurseStoneAggressive is
// WizardCastClanBuffRangeDDCurseStoneAggressive: a Ketra or Varka
// WizardDDMagic2 that petrifies a playable attacker allied with its own
// faction, and a player who helps one, and ignores such a player it sees,
// attacking any other. A hit while its top desire target is someone else
// may draw its range DD magic on the attacker.
func CastClanBuffRangeDDCurseStoneAggressive() script.Script {
	base := ddMagic2Hooks()
	return script.Script{Dir: ddMagic2Dir, Behavior: true, NPCs: []int32{21334, 21342, 21360, 21368}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if defaultnpc.MaybeCastPetrify(n, attacker) {
				return
			}
			if top := n.TopDesireTarget(); top != nil && !script.Same(top, attacker) && s.Rnd(100) < 3 {
				castByType(n, attacker, npc.SkillTypeRangeDD, 1000000)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil || alliedWithFaction(n, p) {
				return
			}
			defaultnpc.TryToAttack(s, n, e.Creature)
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

// alliedWithFaction reports whether p is allied with the Varka or Ketra
// faction n's clans name.
func alliedWithFaction(n *script.NPC, p *script.Player) bool {
	return (p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan")) ||
		(p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan"))
}

// buffOnce makes n, while slot still holds 0, on a draw under 33 above half
// of its HP, want to cast both of its buffs on itself, weight 1000000, and
// sets slot to 1.
func buffOnce(s *script.Script, n *script.NPC, slot script.IntSlot) {
	if n.Int(slot) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 {
		castByType(n, n, npc.SkillTypeBuff1, 1000000)
		castByType(n, n, npc.SkillTypeBuff2, 1000000)
		n.SetInt(slot, 1)
	}
}

// curseOrAttack makes n, on a draw under 33 when c holds no effect of the
// skill its template lists under t, want to cast that skill at target,
// weight 1000000, when it can pay for it at c; otherwise n gives up its
// spells (int slot 0 set to 1) and wants to attack c, weight 1000.
func curseOrAttack(s *script.Script, n *script.NPC, c, target script.Creature, t npc.SkillType) {
	ref, _ := n.SkillByType(t)
	if s.Rnd(100) < 33 && s.AbnormalLevel(c, ref) <= 0 {
		if n.MeetsHPMPConditions(c, ref) {
			n.AddCastDesire(target, ref, 1000000)
		} else {
			n.SetInt(script.SlotIntAI0, 1)
			n.AddAttackDesire(c, 1000)
		}
	}
}
