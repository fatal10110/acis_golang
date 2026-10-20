package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyPrivateCoupleFollowerBabyAggressive is
// PartyPrivateCoupleFollowerBabyAggressive: an aggressive
// PartyPrivateCoupleFollower that buffs itself as it spawns and may strike
// around itself with its self range physical special skill when the
// creature it most wants to act on hits it, and when a clan member is hit
// while it is settled and not attacking.
func PartyPrivateCoupleFollowerBabyAggressive() script.Script {
	base := PartyPrivateCoupleFollowerHooks()
	return script.Script{Dir: partyPrivateCoupleFollowerDir, Behavior: true, NPCs: []int32{22120}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			castByType(e.NPC, e.NPC, npc.SkillTypeBuff)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && script.Same(topDesireTarget, attacker) && s.Rnd(100) < 33 {
					castByType(n, n, npc.SkillTypeSelfRangePhysicalSpecial)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if called.LifeTime() > 7 && e.Attacker.Playable() && called.Intention() != script.IntentionAttack && s.Rnd(100) < 33 {
				castByType(called, called, npc.SkillTypeSelfRangePhysicalSpecial)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// coupleFollowerFemaleBuff makes n want its buff on itself when c holds no
// effect of its kind, a draw under 33 passes and n stands above half its
// HP.
func coupleFollowerFemaleBuff(s *script.Script, n *script.NPC, c script.Creature) {
	buff, ok := n.SkillByType(npc.SkillTypeBuff)
	if s.AbnormalLevel(c, buff) <= 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 && ok {
		n.AddCastDesire(n, buff, 1000000)
	}
}

// PartyPrivateCoupleFollowerFemaleAggressive is
// PartyPrivateCoupleFollowerFemaleAggressive: an aggressive
// PartyPrivateCoupleFollower that buffs itself as it spawns. Hit by the
// creature it most wants to act on, or told of a hit on its party while it
// is settled and not attacking, it may debuff and strike the attacker with
// its physical special skill and buff itself; a hit on a clan member
// draws only the strike and the debuff.
func PartyPrivateCoupleFollowerFemaleAggressive() script.Script {
	base := PartyPrivateCoupleFollowerHooks()
	return script.Script{Dir: partyPrivateCoupleFollowerDir, Behavior: true, NPCs: []int32{22108}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			castByType(e.NPC, e.NPC, npc.SkillTypeBuff)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && script.Same(topDesireTarget, attacker) {
					maybeDebuff(s, n, attacker, npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypePhysicalSpecial)
					}
					coupleFollowerFemaleBuff(s, n, attacker)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypePhysicalSpecial)
				}
				maybeDebuff(s, called, attacker, npc.SkillTypeDebuff)
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if called.LifeTime() > 7 && target.Playable() && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					castByType(called, target, npc.SkillTypePhysicalSpecial)
				}
				maybeDebuff(s, called, target, npc.SkillTypeDebuff)
				coupleFollowerFemaleBuff(s, called, target)
			}
			base.PartyAttacked(s, e)
		},
	})}
}
