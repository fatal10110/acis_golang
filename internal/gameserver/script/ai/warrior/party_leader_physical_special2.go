package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const physicalSpecial2Dir = "ai/individual/Monster/WarriorBase/Warrior/PartyLeaderWarrior/PartyLeaderPhysicalSpecial2"

// The script events a heretic leader broadcasts: a call for help against a
// creature, and the call home once it left its territory.
const (
	eventHereticHelp = 10033
	eventHereticHome = 10035
)

// PartyLeaderPhysicalSpecial2 is PartyLeaderPhysicalSpecial2: a
// PartyLeaderWarrior that, hit by the creature it most wants to attack,
// directly or through a settled clan member, may strike it with its second
// physical special skill when it stands 40 or more away and with its first
// one when it stands closer. It binds no NPC of its own.
func PartyLeaderPhysicalSpecial2() script.Script {
	return script.Script{Dir: physicalSpecial2Dir, Behavior: true, Hooks: physicalSpecial2Hooks()}
}

// physicalSpecial2Hooks are PartyLeaderPhysicalSpecial2's hooks, which its
// children derive theirs from.
func physicalSpecial2Hooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				physicalSpecial2Strikes(s, n, attacker)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if called.LifeTime() > 7 && e.Attacker.Playable() {
				physicalSpecial2Strikes(s, called, e.Attacker)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyLeaderPhysicalSpecial2")
}

// physicalSpecial2Strikes makes n, when attacker is the creature it most
// wants to attack, want its second physical special skill on attacker at a
// 33% chance with attacker 40 or more away, and its first one at a 33%
// chance with attacker closer.
func physicalSpecial2Strikes(s *script.Script, n *script.NPC, attacker script.Creature) {
	topDesireTarget := n.TopDesireTarget()
	if topDesireTarget == nil {
		return
	}
	if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) && n.Distance2D(attacker) >= 40 {
		addCastByType(n, attacker, npc.SkillTypePhysicalSpecial2, 1000000)
	}
	if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) && n.Distance2D(attacker) < 40 {
		addCastByType(n, attacker, npc.SkillTypePhysicalSpecial, 1000000)
	}
}

// helpRange is how far a heretic leader's call for help carries.
func helpRange(n *script.NPC) int { return int(n.IntParamOr("HelpCastRange", 500)) }

// callHelpAgainst broadcasts, unless n is attacking or casting already,
// its call for help against c.
func callHelpAgainst(s *script.Script, n *script.NPC, c script.Creature) {
	if n.Intention() != script.IntentionAttack && n.Intention() != script.IntentionCast {
		s.BroadcastScriptEventEx(n, eventHereticHelp, c.ObjectID(), n.ObjectID(), helpRange(n))
	}
}

// returnHome makes n, out of its territory, drop its attack desires, want
// to walk back to its spawn and call its party home.
func returnHome(s *script.Script, n *script.NPC) {
	n.RemoveAllAttackDesire()
	n.AddMoveToDesire(n.SpawnLocation(), 100)
	s.BroadcastScriptEvent(n, eventHereticHome, n.ObjectID(), int(n.IntParamOr("DistNoDesire", 500)))
}

// PartyLeaderHeretic2Aggressive is PartyLeaderHeretic2Aggressive: a
// PartyLeaderPhysicalSpecial2 that keeps walking home when idle, calls for
// help against what it sees or what hits it unless it fights already, and
// attacks the players and summons it sees, wanting its special attack on
// them and its self heal. Under 40% of its HP it may tell those around
// that a prophet or a cleric hits it or casts near it, and, until its self
// heal has landed, look around again as its party is hit. Out of its
// territory it heads home and calls its party along.
//
// Its memory: IntAI3 is set once its self heal landed.
func PartyLeaderHeretic2Aggressive() script.Script {
	base := physicalSpecial2Hooks()
	return script.Script{Dir: physicalSpecial2Dir, Behavior: true, NPCs: []int32{22155, 22159, 22167}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
			n.AddMoveToDesire(n.SpawnLocation(), 30)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			callHelpAgainst(s, n, c)
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			if !script.Same(c, n) {
				addCastByType(n, c, npc.SkillTypeSpecialAttack, 1000000)
				addCastByType(n, n, npc.SkillTypeSelfMagicHeal, 1000000)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			callHelpAgainst(s, n, attacker)
			if n.HPRatio() < 0.4 && s.Rnd(100) < 33 {
				p := script.ActingPlayer(attacker)
				if p == nil {
					// An attacker with no player behind it has no class to
					// read: the hook stops here.
					return
				}
				classID := p.ClassID()
				isCleric := p.InClassGroup("@cleric_group")
				if s.Rnd(100) < 33 && (classID == 5 || classID == 90 || isCleric) {
					s.BroadcastScriptEventEx(n, 10002, attacker.ObjectID(), n.ObjectID(), helpRange(n))
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if called.HPRatio() < 0.4 && s.Rnd(100) < 33 && called.Int(script.SlotIntAI3) == 0 {
				called.LookNeighbor()
			}
			base.PartyAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if isSkillOfType(e.NPC, e.Skill, npc.SkillTypeSelfMagicHeal) && e.Success {
				e.NPC.SetInt(script.SlotIntAI3, 1)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			if n.HPRatio() < 0.4 && s.Rnd(100) < 33 && e.Caster.InClassGroup("@cleric_group") {
				s.BroadcastScriptEventEx(n, 10002, e.Caster.ObjectID(), n.ObjectID(), helpRange(n))
			}
			base.SeeSpell(s, e)
		},
		OnOutOfTerritory: func(s *script.Script, e script.OutOfTerritory) {
			returnHome(s, e.NPC)
		},
	})}
}

// PartyLeaderHereticAggressive is PartyLeaderHereticAggressive: a
// PartyLeaderPhysicalSpecial2 that calls for help against what it sees or
// what hits it unless it fights already, and attacks the players and
// summons it sees. Its finished casts change nothing. Out of its territory
// it heads home and calls its party along, and as it dies it tells those
// around.
func PartyLeaderHereticAggressive() script.Script {
	base := physicalSpecial2Hooks()
	return script.Script{Dir: physicalSpecial2Dir, Behavior: true, NPCs: []int32{22188}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			callHelpAgainst(s, e.NPC, e.Attacker)
			base.Attacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			callHelpAgainst(s, n, c)
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
		},
		OnUseSkillFinished: func(*script.Script, script.UseSkillFinished) {},
		OnOutOfTerritory: func(s *script.Script, e script.OutOfTerritory) {
			returnHome(s, e.NPC)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			s.BroadcastScriptEventEx(e.NPC, 2, 0, e.NPC.ObjectID(), 4000)
		},
	})}
}
