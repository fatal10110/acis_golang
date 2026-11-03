package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const (
	castingEnchantLabClan1Dir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastingEnchantLabClan1"
	castingEnchantLabClan2Dir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastingEnchantLabClan1/WarriorCastingEnchantLabClan2"
)

// CastingEnchantLabClan1 is WarriorCastingEnchantLabClan1: a Warrior that
// wants its first buff on every clan member a creature hits.
func CastingEnchantLabClan1() script.Script {
	return script.Script{Dir: castingEnchantLabClan1Dir, Behavior: true, NPCs: []int32{27318}, Hooks: castingEnchantLabClan1Hooks()}
}

// castingEnchantLabClan1Hooks are WarriorCastingEnchantLabClan1's hooks,
// which its children derive theirs from.
func castingEnchantLabClan1Hooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			// Its four buffs all read BUFF1, and its first two level tests
			// (at most 0, at least 0) between them pass at any level the
			// member holds it at, so the member always gets BUFF1.
			addCastByType(e.Called, e.Caller, npc.SkillTypeBuff1, 1000000)
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCastingEnchantLabClan1")
}

// CastingEnchantLabClan2 is WarriorCastingEnchantLabClan2: a
// WarriorCastingEnchantLabClan1 that, hit by a player or summon whose
// party holds 8 or more, wants its second self buff unless it holds it;
// by one whose party holds 6 or 7, its first. It binds no NPC of its own.
func CastingEnchantLabClan2() script.Script {
	return script.Script{Dir: castingEnchantLabClan2Dir, Behavior: true, Hooks: castingEnchantLabClan2Hooks()}
}

// castingEnchantLabClan2Hooks are WarriorCastingEnchantLabClan2's hooks,
// which its children derive theirs from.
func castingEnchantLabClan2Hooks() script.Hooks {
	base := castingEnchantLabClan1Hooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if p := script.ActingPlayer(attacker); p != nil {
				if members := p.PartyMembers(); members != nil {
					if len(members) >= 8 {
						if selfBuff2, _ := n.SkillByType(npc.SkillTypeSelfBuff2); s.AbnormalLevel(n, selfBuff2) <= 0 {
							n.AddCastDesire(attacker, selfBuff2, 1000000)
						}
					} else if len(members) >= 6 {
						if selfBuff1, _ := n.SkillByType(npc.SkillTypeSelfBuff1); s.AbnormalLevel(n, selfBuff1) <= 0 {
							n.AddCastDesire(attacker, selfBuff1, 1000000)
						}
					}
				}
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorCastingEnchantLabClan2")
}

// PhysicalSpecialLabAggressive is WarriorPhysicalSpecialLabAggressive: a
// WarriorCastingEnchantLabClan2 that attacks the players and summons it
// sees and, on a draw under 33, wants its self-centred physical special
// skill on whatever hits it.
func PhysicalSpecialLabAggressive() script.Script {
	base := castingEnchantLabClan2Hooks()
	return script.Script{Dir: castingEnchantLabClan2Dir, Behavior: true, NPCs: []int32{22059, 22064, 22065, 22076, 22077, 22078}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if s.Rnd(100) < 33 {
				addCastByType(e.NPC, e.Attacker, npc.SkillTypeSelfRangePhysicalSpecial, 1000000)
			}
			base.Attacked(s, e)
		},
	})}
}

// PhysicalSpecialLabRunAggressive is WarriorPhysicalSpecialLabRunAggressive:
// a WarriorCastingEnchantLabClan2 that attacks the players and summons it
// sees and, below 40% of its HP, wants its physical special skill on
// whatever hits it and, out of its territory, to walk back to where it
// spawned.
func PhysicalSpecialLabRunAggressive() script.Script {
	base := castingEnchantLabClan2Hooks()
	return script.Script{Dir: castingEnchantLabClan2Dir, Behavior: true, NPCs: []int32{22063}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.HPRatio() < 0.4 {
				addCastByType(n, e.Attacker, npc.SkillTypePhysicalSpecial, 1000000)
				if !n.InMyTerritory() {
					n.AddMoveToDesire(n.SpawnLocation(), 1000000)
				}
			}
			base.Attacked(s, e)
		},
	})}
}
