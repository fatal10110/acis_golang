package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const coupleCaptainDir = "ai/individual/Monster/WarriorBase/Warrior/PartyLeaderWarrior/PartyLeaderCoupleCaptain"

// PartyLeaderCoupleCaptain is PartyLeaderCoupleCaptain: a
// PartyLeaderWarrior that may pull a player or summon hitting it from afar
// to itself with its summon magic and attack it there. When one of its
// privates dies while it still has a target and more than 70% of its HP, it
// gives way to its silhouette, a private of its own set on that target. It
// binds no NPC of its own.
//
// Its memory: Creature0 is the last player or summon that hit it.
func PartyLeaderCoupleCaptain() script.Script {
	return script.Script{Dir: coupleCaptainDir, Behavior: true, Hooks: coupleCaptainHooks()}
}

// coupleCaptainHooks are PartyLeaderCoupleCaptain's hooks, which its
// children derive theirs from.
func coupleCaptainHooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				n.SetSlot(script.SlotCreature0, attacker)
				i6 := s.Rnd(100)
				if (n.Distance2D(attacker) > 300 && i6 < 20) || (n.Distance2D(attacker) > 100 && i6 < 50) {
					addCastByType(n, attacker, npc.SkillTypeSummonMagic, 1000000)
				}
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if c0 := n.Slot(script.SlotCreature0); e.Success && c0 != nil && isSkillOfType(n, e.Skill, npc.SkillTypeSummonMagic) {
				c0.InstantTeleportTo(n)
				n.AddAttackDesire(c0, 100000)
			}
			base.UseSkillFinished(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			called := e.Called
			if !script.Same(called, e.Caller) && called.TopDesireTarget() != nil && called.HPRatio() > 0.7 {
				at := script.LocOf(called)
				params := script.Params{P1: 1000, P2: called.TopDesireTarget().ObjectID()}
				s.Spawner().CreateOnePrivateEx(called, called.IntParam("silhouette"), script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, params)
				called.DeleteMe()
			}
		},
	}).Named("PartyLeaderCoupleCaptain")
}

// PartyLeaderCoupleCaptainMaleAggressive is
// PartyLeaderCoupleCaptainMaleAggressive: a PartyLeaderCoupleCaptain that
// attacks the players and summons it sees and, on a hit by the creature it
// most wants to attack, may strike it with its physical special skill and
// around itself with its self-centred one. Settled and not yet attacking,
// it answers its clan's attacker with the first and its party's attacker
// with both.
func PartyLeaderCoupleCaptainMaleAggressive() script.Script {
	base := coupleCaptainHooks()
	return script.Script{Dir: coupleCaptainDir, Behavior: true, NPCs: []int32{22109}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
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
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, attacker, npc.SkillTypePhysicalSpecial, 1000000)
					}
					if s.Rnd(100) < 33 {
						addCastByType(n, attacker, npc.SkillTypeSelfRangePhysicalSpecial, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && s.Rnd(100) < 33 {
				addCastByType(called, e.Attacker, npc.SkillTypePhysicalSpecial, 1000000)
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if target.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					addCastByType(called, target, npc.SkillTypePhysicalSpecial, 1000000)
				}
				if s.Rnd(100) < 33 {
					addCastByType(called, target, npc.SkillTypeSelfRangePhysicalSpecial, 1000000)
				}
				base.PartyAttacked(s, e)
			}
		},
	})}
}

// PartyLeaderCoupleCaptainNurseAggressive is
// PartyLeaderCoupleCaptainNurseAggressive: a PartyLeaderCoupleCaptain that
// attacks the players and summons it sees. Hit by the creature it most
// wants to attack, by itself or through a settled clan member, it may cast
// its self-centred DD magic when that creature stands close and its DD
// magic on it when it stands away.
func PartyLeaderCoupleCaptainNurseAggressive() script.Script {
	base := coupleCaptainHooks()
	return script.Script{Dir: coupleCaptainDir, Behavior: true, NPCs: []int32{22118}, Hooks: base.With(script.Hooks{
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
				nurseCasts(s, n, attacker)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if called.LifeTime() > 7 && e.Attacker.Playable() {
				nurseCasts(s, called, e.Attacker)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// nurseCasts makes n, when attacker is the creature it most wants to
// attack, want its self-centred DD magic on itself at a 33% chance with
// attacker under 200 away, and its DD magic on attacker at a 20% chance
// with attacker over 100 away.
func nurseCasts(s *script.Script, n *script.NPC, attacker script.Creature) {
	topDesireTarget := n.TopDesireTarget()
	if topDesireTarget == nil {
		return
	}
	if n.Distance2D(attacker) < 200 && s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
		addCastByType(n, n, npc.SkillTypeSelfRangeDDMagic, 1000000)
	}
	if n.Distance2D(attacker) > 100 && s.Rnd(100) < 20 && script.Same(topDesireTarget, attacker) {
		addCastByType(n, topDesireTarget, npc.SkillTypeDDMagic, 1000000)
	}
}
