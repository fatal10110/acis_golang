package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// QueenAntPrivateRoyalGuardAnt is QueenAntPrivateRoyalGuardAnt: Queen Ant's
// royal guard, a PartyPrivateWarrior that curses an attacker more than 8
// levels above it and drops its hate, and mutes such a caster. Any other
// hit leaves it unmoved.
func QueenAntPrivateRoyalGuardAnt() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{29005}, Hooks: base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
				n.StopHate(attacker)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if e.Caster.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

// healMasterAndBomb makes n, under 40% of its HP with a live master, want
// to heal that master and blow itself up, on a draw under 33.
func healMasterAndBomb(s *script.Script, n *script.NPC) {
	if s.Rnd(100) < 33 && n.HPRatio() < 0.4 {
		if master := n.Master(); master != nil && !master.Dead() {
			castByType(n, master, npc.SkillTypeMagicHeal)
			castByType(n, n, npc.SkillTypeBomb)
		}
	}
}

// RoyalRushPartyPrivateBomb is RoyalRushPartyPrivateBomb: a
// PartyPrivateWarrior that may strike the player it most wants to attack
// with its physical special skill and, low on HP, heal its master and blow
// itself up.
func RoyalRushPartyPrivateBomb() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21437}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && script.Same(topDesireTarget, attacker) {
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypePhysicalSpecial1)
					}
					healMasterAndBomb(s, n)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			healMasterAndBomb(s, e.Called)
			base.PartyAttacked(s, e)
		},
	})}
}

// RoyalRushPartyPrivateHate is RoyalRushPartyPrivateHate: a
// PartyPrivateWarrior that buffs itself as it appears and may draw the
// hate of the player it most wants to attack, or of a party member's
// attacker.
func RoyalRushPartyPrivateHate() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21435}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			castByType(e.NPC, e.NPC, npc.SkillTypeSelfBuff)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
					castByType(n, attacker, npc.SkillTypeMobHate)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if s.Rnd(100) < 10 {
				castByType(e.Called, e.Target, npc.SkillTypeMobHate)
			}
			base.PartyAttacked(s, e)
		},
	})}
}
