package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyPrivateCouplePhysicalSpecial is PartyPrivateCouplePhysicalSpecial:
// a PartyPrivateWarrior that strikes once with its physical special skill,
// at the player it most wants to attack once under 20% of its HP, or at a
// clan member's attacker. When a party member dies within 100 of it, it
// heals and buffs itself and turns all its hate on the creature it most
// wants to attack.
func PartyPrivateCouplePhysicalSpecial() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21433}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				if script.Same(n.TopDesireTarget(), attacker) && n.HPRatio() < 0.2 && s.Rnd(100) < 33 {
					strikeOnce(n, attacker)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 33 {
				strikeOnce(called, e.Attacker)
			}
			base.ClanAttacked(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(caller, called) && called.Distance2D(caller) < 100 {
				addCastByType(called, called, npc.SkillTypeMagicHeal, 1000000)
				addCastByType(called, called, npc.SkillTypeSelfBuff, 1000000)
				if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil {
					called.RemoveAllAttackDesire()
					called.AddAttackDesire(topDesireTarget, 1000)
				}
			}
			base.PartyDied(s, e)
		},
	})}
}

// PartyPrivateCoward is PartyPrivateCoward: a PartyPrivateWarrior that may
// strike its most hated attacker with its physical special skill, and a
// clan or party member's attacker while it is not attacking, and runs
// from where its master died.
func PartyPrivateCoward() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21313}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil {
					if s.Rnd(100) < 33 && script.Same(mostHated, attacker) {
						castByType(n, attacker, npc.SkillTypePhysicalSpecial)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if called.LifeTime() > 7 && e.Attacker.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, e.Attacker, npc.SkillTypePhysicalSpecial)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if called.LifeTime() > 7 && e.Target.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, e.Target, npc.SkillTypePhysicalSpecial)
				}
			}
			base.PartyAttacked(s, e)
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if master := e.Called.Master(); master != nil && script.Same(e.Caller, master) {
				e.Called.AddFleeDesire(e.Caller, 500, 10000000)
			}
		},
	})}
}

// PartyPrivatePabelgolem is PartyPrivatePabelgolem: a PartyPrivateWarrior
// that may cast its DD magic on whatever hits it.
func PartyPrivatePabelgolem() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22061, 22075}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if s.Rnd(100) < 33 {
				castByType(e.NPC, e.Attacker, npc.SkillTypeDDMagic)
			}
			base.Attacked(s, e)
		},
	})}
}

// PartyPrivatePhysicalStone is PartyPrivatePhysicalStone: a Ketra or Varka
// PartyPrivateWarrior that petrifies an attacker allied with its own
// faction, and a player who helps one. Otherwise it may strike its most
// hated attacker with its physical special skill.
func PartyPrivatePhysicalStone() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21344, 21346, 21374, 21348, 21370, 21372}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if defaultnpc.MaybeCastPetrify(n, attacker) {
					return
				}
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypePhysicalSpecial)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}
