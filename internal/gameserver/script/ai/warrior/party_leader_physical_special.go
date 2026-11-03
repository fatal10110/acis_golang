package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderPhysicalSpecial is PartyLeaderPhysicalSpecial: a
// PartyLeaderWarrior that strikes once with its physical special skill,
// either at the player it most wants to attack while under 20% of its own
// HP, or at a clan member's attacker once settled.
func PartyLeaderPhysicalSpecial() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{20947, 20989, 20974, 20767, 22080, 22096}, Hooks: PartyLeaderPhysicalSpecialHooks()}
}

// strikeOnce makes n want to cast its template's physical special skill on
// target and marks the strike done in i_ai0, whether or not the template
// lists such a skill.
func strikeOnce(n *script.NPC, target script.Creature) {
	addCastByType(n, target, npc.SkillTypePhysicalSpecial, 1000000)
	n.SetInt(script.SlotIntAI0, 1)
}

// PartyLeaderPhysicalSpecialHooks are PartyLeaderPhysicalSpecial's hooks,
// which its children derive theirs from.
func PartyLeaderPhysicalSpecialHooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && script.Same(topDesireTarget, attacker) {
					if n.Int(script.SlotIntAI0) == 0 && n.HPRatio() < 0.2 && s.Rnd(100) < 33 {
						strikeOnce(n, attacker)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 {
				if called.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 33 {
					strikeOnce(called, e.Attacker)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyLeaderPhysicalSpecial")
}

// PartyLeaderPhysicalSpecialAggressive is
// PartyLeaderPhysicalSpecialAggressive: a PartyLeaderPhysicalSpecial that
// attacks the players and summons it sees, and, settled in its territory
// and not yet attacking, may first spend its strike on one standing away.
func PartyLeaderPhysicalSpecialAggressive() script.Script {
	base := PartyLeaderPhysicalSpecialHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: partyLeaderPhysicalSpecialAggressiveIDs(), Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if n.Distance2D(c) > 100 && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 33 {
					strikeOnce(n, c)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}

// partyLeaderPhysicalSpecialAggressiveIDs are
// PartyLeaderPhysicalSpecialAggressive's own NPCs.
func partyLeaderPhysicalSpecialAggressiveIDs() []int32 {
	return []int32{
		20983, 20959, 20758, 20994, 20950, 20956, 21075, 20941, 20939, 20991, 20398, 20738, 20745, 20755,
		20763, 20953, 20980, 22084, 22123, 22135,
	}
}

// PartyLeaderCouplePhysicalSpecial is PartyLeaderCouplePhysicalSpecial: a
// PartyLeaderWarrior that strikes once with its physical special skill as
// PartyLeaderPhysicalSpecial does, though it answers a clan member whatever
// it is doing. When one of its privates dies within 100 of it, it heals and
// buffs itself and turns all its hate on the creature it most wants to
// attack.
func PartyLeaderCouplePhysicalSpecial() script.Script {
	base := PartyLeaderHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21432}, Hooks: base.With(script.Hooks{
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
			called := e.Called
			if called.IsMaster() && e.Caller.Distance2D(called) < 100 {
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
