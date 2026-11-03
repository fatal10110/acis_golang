package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// HereticPriestAggressive is WarriorHereticPriestAggressive: a Warrior of
// the Pagan Temple that keeps walking home when idle and calls for help
// against what it sees or what hits it unless it fights already. It
// strikes what it sees, and what hits it while it most wants to attack
// it, with its first physical special within 40 and its second one under
// 500, and attacks the players and summons it sees in its territory. It
// may want to bind a paladin, a phoenix knight or a cleric it sees, and it may send a player
// it sees in a party larger than its party_members parameter, with the
// player's summon, to one of its three teleport points. A finished cast
// turns it on the playable it targeted. Out of its territory it heads home
// and calls those around home, unless it holds its ground (Hold).
func HereticPriestAggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22142, 22146, 22175}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			callHelpAgainst(s, n, attacker)
			topDesireTarget := n.TopDesireTarget()
			if n.IntParam("Hold") == 0 {
				if attacker.Playable() && topDesireTarget != nil && script.Same(topDesireTarget, attacker) {
					priestStrikeTop(s, n, attacker, topDesireTarget, false)
				}
				// The parent hears the hit with no skill.
				plain := e
				plain.Skill = skill.Ref{}
				base.Attacked(s, plain)
			} else if attacker.Playable() && topDesireTarget != nil {
				if script.Same(topDesireTarget, attacker) {
					priestStrikeTop(s, n, attacker, topDesireTarget, true)
				}
				n.AddAttackDesireHold(attacker, 100)
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			topDesireTarget := called.TopDesireTarget()
			if called.IntParam("Hold") == 0 {
				if attacker.Playable() {
					if topDesireTarget != nil {
						if script.Same(topDesireTarget, attacker) {
							priestStrikeTop(s, called, attacker, topDesireTarget, false)
						}
					} else {
						priestStrike(s, called, attacker, false)
					}
				}
				base.ClanAttacked(s, e)
			} else if called.LifeTime() > 7 && attacker.Playable() && !fighting(called.Intention()) {
				priestStrike(s, called, attacker, true)
				called.AddAttackDesireHold(attacker, 100)
			} else if topDesireTarget != nil && !script.Same(attacker, topDesireTarget) && called.Distance2D(topDesireTarget) > 500 {
				called.RemoveAttackDesire(topDesireTarget)
				priestStrike(s, called, attacker, true)
			}
			if attacker.Playable() {
				called.AddAttackDesireHold(attacker, 100)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			callHelpAgainst(s, n, c)
			if n.Distance2D(c) <= 40 {
				addCastHoldByType(n, c, npc.SkillTypePhysicalSpecial, 1000000)
			} else if n.Distance2D(c) < 500 {
				addCastHoldByType(n, c, npc.SkillTypePhysicalSpecial2, 1000000)
			}
			if !c.Playable() || n.Dead() {
				return
			}
			if n.IntParam("Hold") == 1 {
				if n.InMyTerritory() {
					n.AddAttackDesireHold(c, 200)
				}
			} else if n.InMyTerritory() {
				n.AddAttackDesire(c, 200)
			}
			i0 := s.Rnd(100)
			acting := script.ActingPlayer(c)
			if acting == nil {
				// A playable with no player behind it has no class to
				// read: the hook stops here.
				return
			}
			playerClassID := acting.ClassID()
			isInClericGroup := acting.InClassGroup("@cleric_group")
			p, isPlayer := c.(*script.Player)
			if i0 < 33 && isPlayer && (playerClassID == 5 || playerClassID == 90 || isInClericGroup) {
				addCastHoldByType(n, c, npc.SkillTypeHoldMagic, 1000000)
				priestStrike(s, n, c, n.IntParam("Hold") != 0)
			}
			i0 = s.Rnd(100)
			if i0 < 33 && isPlayer {
				if members := p.PartyMembers(); members != nil && len(members) > int(n.IntParam("party_members")) && n.IntParam("NoPCTeleport") == 0 && n.InSpawnTerritory(c) {
					sendAway(s, n, p)
				}
			}
			base.SeeCreature(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if e.Creature.Playable() {
				if e.NPC.IntParam("Hold") == 1 {
					e.NPC.AddAttackDesireHold(e.Creature, 100)
				} else {
					e.NPC.AddAttackDesire(e.Creature, 100)
				}
			}
		},
		OnOutOfTerritory: func(s *script.Script, e script.OutOfTerritory) {
			n := e.NPC
			if n.IntParam("Hold") == 0 {
				n.RemoveAllAttackDesire()
				n.AddMoveToDesire(n.SpawnLocation(), 100)
				s.BroadcastScriptEvent(n, eventHereticHome, 0, int(n.IntParamOr("DistNoDesire", 500)))
			}
		},
	})}
}

// priestStrike makes n want, on c, its first physical special skill at a
// 33% chance with c within 40, and its second one with c under 500; held
// in place when hold is set.
func priestStrike(s *script.Script, n *script.NPC, c script.Creature, hold bool) {
	cast := addCastByType
	if hold {
		cast = addCastHoldByType
	}
	if n.Distance2D(c) <= 40 {
		if s.Rnd(100) < 33 {
			cast(n, c, npc.SkillTypePhysicalSpecial, 1000000)
		}
	} else if n.Distance2D(c) < 500 {
		cast(n, c, npc.SkillTypePhysicalSpecial2, 1000000)
	}
}

// priestStrikeTop is priestStrike on attacker, the creature n most wants
// to attack, top; with attacker 500 or more away n drops its attack
// desire on top instead, and strikes nothing at that reach.
func priestStrikeTop(s *script.Script, n *script.NPC, attacker, top script.Creature, hold bool) {
	if n.Distance2D(attacker) >= 500 {
		n.RemoveAttackDesire(top)
	}
	priestStrike(s, n, attacker, hold)
}

// sendAway teleports p, and its summon after it, to one of n's three
// teleport points, picked at random.
func sendAway(s *script.Script, n *script.NPC, p *script.Player) {
	c0 := p.ActiveSummon()
	i1 := s.Rnd(3)
	telPosX1, telPosX2, telPosX3 := n.IntParam("TelPosX1"), n.IntParam("TelPosX2"), n.IntParam("TelPosX3")
	telPosY1, telPosY2, telPosY3 := n.IntParam("TelPosY1"), n.IntParam("TelPosY2"), n.IntParam("TelPosY3")
	telPosZ1, telPosZ2, telPosZ3 := n.IntParam("TelPosZ1"), n.IntParam("TelPosZ2"), n.IntParam("TelPosZ3")
	var to script.Loc
	switch i1 {
	case 0:
		to = script.Loc{X: int(telPosX1), Y: int(telPosY1), Z: int(telPosZ1)}
	case 1:
		to = script.Loc{X: int(telPosX2), Y: int(telPosY2), Z: int(telPosZ2)}
	default:
		to = script.Loc{X: int(telPosX3), Y: int(telPosY3), Z: int(telPosZ3)}
	}
	p.TeleportTo(to, 0)
	if c0 != nil {
		script.TeleportTo(c0, to)
	}
}

// addCastHoldByType makes n want to cast its template's skill of type t on
// target with weight, held in place; a template that lists none casts
// nothing.
func addCastHoldByType(n *script.NPC, target script.Creature, t npc.SkillType, weight float64) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesireHold(target, ref, weight)
	}
}
