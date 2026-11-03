package wizard

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderCastRangeCurseStoneAggressive is
// PartyLeaderWizardCastRangeCurseStoneAggressive: a Ketra or Varka
// PartyLeaderWizardDD2 that petrifies a playable attacker allied with its
// own faction, and a player who helps one, and ignores such a player it
// sees, attacking any other. Above half of its HP it buffs itself once on
// the first playable hit, its own or its clan's, and it may curse the
// creatures around it.
func PartyLeaderCastRangeCurseStoneAggressive() script.Script {
	base := partyLeaderDD2Hooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21347, 21373}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if defaultnpc.MaybeCastPetrify(n, attacker) {
					return
				}
				selfBuffOnce(n)
				if top := n.TopDesireTarget(); top != nil && !script.Same(top, attacker) {
					maybeRangeCurse(s, n, attacker)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				if defaultnpc.MaybeCastPetrify(called, attacker) {
					return
				}
				if called.LifeTime() > 7 {
					selfBuffOnce(called)
					maybeRangeCurse(s, called, attacker)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			p := script.ActingPlayer(e.Creature)
			if p == nil {
				return
			}
			if p.AlliedWithVarka() && slices.Contains(n.Clans(), "varka_silenos_clan") {
				return
			}
			if p.AlliedWithKetra() && slices.Contains(n.Clans(), "ketra_orc_clan") {
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

// selfBuffOnce makes n, the first time it is asked (int slot 3 still 0)
// while above half of its HP, want to cast both of its buffs on itself.
func selfBuffOnce(n *script.NPC) {
	if n.Int(script.SlotIntAI3) == 0 && n.HPRatio() > 0.5 {
		castByType(n, n, npc.SkillTypeBuff1, 9999999)
		castByType(n, n, npc.SkillTypeBuff2, 9999999)
		n.SetInt(script.SlotIntAI3, 1)
	}
}

// maybeRangeCurse makes n, on a draw under 33 when c holds no effect of its
// range debuff yet, want to cast that debuff around itself when it can pay
// for it at c; otherwise n gives up its spells (int slot 0 set to 1) and
// wants to attack c, weight 1000.
func maybeRangeCurse(s *script.Script, n *script.NPC, c script.Creature) {
	rangeDebuff, _ := n.SkillByType(npc.SkillTypeRangeDebuff)
	if s.Rnd(100) < 33 && s.AbnormalLevel(c, rangeDebuff) <= 0 {
		if n.MeetsHPMPConditions(c, rangeDebuff) {
			n.AddCastDesire(n, rangeDebuff, 9999999)
		} else {
			n.SetInt(script.SlotIntAI0, 1)
			n.AddAttackDesire(c, 1000)
		}
	}
}
