// Package warrior holds the Warrior family of monster behaviors:
// WarriorBase, Warrior and WarriorAggressive.
package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
)

// Base is WarriorBase. It binds no NPC of its own.
func Base() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase", Behavior: true, Hooks: BaseHooks()}
}

// The script events a warrior sends and answers: an ally under attack
// calls for help, a shouting warrior names its target, and a caller hands
// its target over.
const (
	eventHelp        = 10001
	eventShoutTarget = 10016
	eventTarget      = 10020
)

// isPlayer reports whether c is a player.
func isPlayer(c script.Creature) bool {
	_, ok := c.(*script.Player)
	return ok
}

// fighting reports whether i is an attack or a cast.
func fighting(i script.Intention) bool {
	return i == script.IntentionAttack || i == script.IntentionCast
}

// BaseHooks are WarriorBase's hooks, which the Warrior behaviors derive
// theirs from.
func BaseHooks() script.Hooks {
	base := monster.Hooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1") {
				if n.IntParam("AttackLowLevel") == 1 {
					n.LookNeighbor()
				}
			} else if strings.EqualFold(e.Name, "2") {
				if n.IntParam("IsVs") == 1 {
					n.SetSlot(script.SlotCreature0, n)
				}
			}
			return base.Timer(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.IntParam("IsVs") == 1 {
				n.SetSlot(script.SlotCreature0, n)
			}
			if n.Int(script.SlotParam1) == 1000 {
				if c0 := s.Object(n.Int(script.SlotParam2)); c0 != nil {
					n.AddCastDesire(c0, skill.Ref{ID: 4663, Level: 1}, 10000)
					n.AddAttackDesire(c0, 500)
				}
			}
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			attacked(s, e)
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			mostHated := called.MostHated()
			if mostHated != nil && !script.Same(mostHated, attacker) && called.IntParam("AttackLowHP") == 1 && attacker.HPRatio() < 0.3 && s.Rnd(100) < 3 {
				called.RemoveAllAttackDesire()
				if attacker.Playable() {
					called.AddAttackDesire(attacker, 100)
				}
			}
			if called.IntParam("AttackLowLevel") == 1 && !fighting(called.Intention()) {
				s.StartTimer("1", called, nil, 7000*time.Millisecond)
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if seeCreature(s, e) {
				base.SeeCreature(s, e)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			seeSpell(s, e)
			base.SeeSpell(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.Dead() {
				return
			}
			switch e.EventID {
			case eventHelp:
				if n.Intention() != script.IntentionAttack && !script.Same(n.Slot(script.SlotCreature0), s.Object(e.Arg1)) {
					switch s.Rnd(3) {
					case 0:
						n.Say(script.NpcString(1000392))
					case 1:
						n.Say(script.NpcString(1000393))
					case 2:
						n.Say(script.NpcString(1000394))
					}
					n.AddAttackDesire(s.Object(e.Arg1), 1000000)
				}
			case eventShoutTarget:
				if s.Rnd(100) < 50 {
					n.RemoveAllAttackDesire()
					if c0 := s.Object(e.Arg1); c0 != nil && c0.Playable() {
						n.AddAttackDesire(c0, 100)
					}
				}
			case eventTarget:
				if c0 := s.Object(e.Arg1); c0 != nil && c0.Playable() {
					n.AddAttackDesire(c0, 100)
				}
			}
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if selfExplosion, ok := e.NPC.SkillByType(npc.SkillTypeSelfExplosion); ok && e.Skill.ID == selfExplosion.ID {
				e.NPC.DoDie(e.NPC)
			}
		},
	})
}

// attacked is WarriorBase's attacked hook up to its parent call.
func attacked(s *script.Script, e script.Attacked) {
	n, attacker := e.NPC, e.Attacker
	if n.IntParam("CreviceOfDiminsion") != 0 && !n.InSpawnTerritory(attacker) {
		n.StopHate(attacker)
		return
	}
	if !n.Stunned() && !n.Paralyzed() {
		if ssRate := n.IntParam("SoulShotRate"); ssRate != 0 && int32(s.Rnd(100)) < ssRate {
			n.RechargeShots(true, false)
		}
		if spsRate := n.IntParam("SpiritShotRate"); spsRate != 0 && int32(s.Rnd(100)) < spsRate {
			n.RechargeShots(false, true)
		}
	}

	mostHated := n.MostHated()
	intention := n.Intention()
	hpRatio := n.HPRatio()
	attackerHPRatio := attacker.HPRatio()

	if n.IntParam("AttackLowLevel") == 1 && !fighting(intention) {
		s.StartTimer("1", n, nil, 7000*time.Millisecond)
	}

	if mostHated != nil && !script.Same(mostHated, attacker) {
		if n.IntParam("AttackLowHP") == 1 && attackerHPRatio <= 0.3 && s.Rnd(100) < 10 {
			n.RemoveAllAttackDesire()
			if attacker.Playable() {
				n.AddAttackDesire(attacker, 1*100)
			}
			switch s.Rnd(3) {
			case 0:
				n.Say(script.NpcString(1000307))
			case 1:
				n.Say(script.NpcString(1000427))
			case 2:
				n.Say(script.NpcString(1000428))
			}
		}

		if n.IntParam("IsVs") == 1 && isPlayer(attacker) && script.Same(n.Slot(script.SlotCreature0), n) {
			switch s.Rnd(5) {
			case 0:
				n.Say(script.NpcString(1000288, attacker.Name()))
			case 1:
				n.Say(script.NpcString(1000388, attacker.Name()))
			case 2:
				n.Say(script.NpcString(1000389))
			case 3:
				n.Say(script.NpcString(1000390))
			case 4:
				n.Say(script.NpcString(1000391))
			}
			n.SetSlot(script.SlotCreature0, attacker)
			s.StartTimer("2", n, nil, 20000*time.Millisecond)
			s.BroadcastScriptEvent(n, eventHelp, attacker.ObjectID(), 600)
		}
	}

	if specialSkill, ok := n.SkillByType(npc.SkillTypeSpecialSkill); ok && hpRatio <= 0.3 && s.Rnd(100) < 10 && s.AbnormalLevel(n, specialSkill) <= 0 {
		switch s.Rnd(4) {
		case 0:
			n.Say(script.NpcString(1000290))
		case 1:
			n.Say(script.NpcString(1000395))
		case 2:
			n.Say(script.NpcString(1000396))
		case 3:
			n.Say(script.NpcString(1000397))
		}
		n.AddCastDesire(n, specialSkill, 1000000)
	}

	if isPlayer(attacker) {
		if helpHeroSilhouetteID := n.IntParam("HelpHeroSilhouette"); helpHeroSilhouetteID != 0 && attackerHPRatio <= 0.2 && s.Rnd(100) < 3 {
			at := script.LocOf(n)
			at.X, at.Y = at.X+80, at.Y+80
			s.Spawner().CreateOnePrivateEx(n, helpHeroSilhouetteID, at, 0, script.Params{P3: n.ObjectID()})
		}
	}

	if mostHated != nil && script.Same(mostHated, attacker) && n.IntParam("ShoutTarget") != 0 && s.Rnd(100) < 5 {
		s.BroadcastScriptEvent(n, eventShoutTarget, attacker.ObjectID(), 300)
	}

	if selfExplosion, ok := n.SkillByType(npc.SkillTypeSelfExplosion); ok && hpRatio < 0.5 && s.Rnd(100) < 5 && 10-int(hpRatio*10) > s.Rnd(100) {
		n.AddCastDesire(n, selfExplosion, 1000000)
	}

	if isTransform := n.IntParam("IsTransform"); isTransform > 0 {
		transform(s, n, attacker, isTransform, hpRatio)
	}
}

// transform replaces the NPC, at its transformation step, with the NPC of
// its next step, which turns on attacker.
func transform(s *script.Script, n *script.NPC, attacker script.Creature, isTransform int32, hpRatio float64) {
	step := n.Int(script.SlotParam3)
	next := func(param string, says [3]int32) {
		switch s.Rnd(3) {
		case 0:
			n.Say(script.NpcString(says[0]))
		case 1:
			n.Say(script.NpcString(says[1]))
		case 2:
			n.Say(script.NpcString(says[2]))
		}
		s.Spawner().CreateOnePrivateEx(n, n.IntParam(param), script.LocOf(n), 0, script.Params{P1: 1000, P2: attacker.ObjectID(), P3: step + 1})
		n.DeleteMe()
	}
	switch step {
	case 0:
		if step < isTransform && hpRatio < 0.7 && hpRatio > 0.5 && s.Rnd(100) < 30 {
			next("step1", [3]int32{1000406, 1000407, 1000408})
		}
	case 1:
		if step < isTransform && hpRatio < 0.5 && hpRatio > 0.3 && s.Rnd(100) < 20 {
			next("step2", [3]int32{1000409, 1000410, 1000411})
		}
	case 2:
		if step < isTransform && hpRatio < 0.3 && hpRatio > 0.05 && s.Rnd(100) < 10 {
			next("step3", [3]int32{1000412, 1000413, 1000414})
		}
	}
}

// seeCreature is WarriorBase's see-creature hook up to its parent call;
// false when the hook returns before it.
func seeCreature(s *script.Script, e script.SeeCreature) bool {
	n, c := e.NPC, e.Creature
	if !c.Playable() || n.Dead() {
		return false
	}
	intention := n.Intention()

	switch n.IntParam("HalfAggressive") {
	case 1:
		if !fighting(intention) && s.GameHour() >= 5 {
			defaultnpc.TryToAttack(s, n, c)
		}
		return false
	case 2:
		if !fighting(intention) && s.GameHour() < 5 {
			defaultnpc.TryToAttack(s, n, c)
		}
		return false
	default:
		if randomAggressive := n.IntParam("RandomAggressive"); randomAggressive > 0 {
			if isPlayer(c) && int32(s.Rnd(100)) < randomAggressive {
				if !fighting(intention) {
					defaultnpc.TryToAttack(s, n, c)
				}
				return false
			} else if !fighting(intention) {
				n.RemoveAllAttackDesire()
			}
		}
	}

	if n.IntParam("AttackLowLevel") == 1 && fighting(intention) && n.InRadius2D(c, 300) {
		if c.Level()+15 < n.Level() {
			n.RemoveAllAttackDesire()
			n.AddAttackDesire(c, 7*100)
		}
		s.StartTimer("1", n, nil, 7000*time.Millisecond)
	}

	if isPlayer(c) && !fighting(intention) {
		if n.IntParam("IsVs") == 1 {
			levelDifference := c.Level() - n.Level()
			if levelDifference < 0 {
				levelDifference = -levelDifference
			}
			if levelDifference <= 2 {
				switch s.Rnd(5) {
				case 0:
					n.Say(script.NpcString(1000287, c.Name()))
				case 1:
					n.Say(script.NpcString(1000384, c.Name()))
				case 2:
					n.Say(script.NpcString(1000385, c.Name()))
				case 3:
					n.Say(script.NpcString(1000386, c.Name()))
				case 4:
					n.Say(script.NpcString(1000387, c.Name()))
				}
				defaultnpc.TryToAttack(s, n, c)
			}
		}

		if n.IntParam("DaggerBackAttack") == 1 && s.Rnd(100) < 50 && n.InRadius2D(c, 100) && n.Behind(c) {
			switch s.Rnd(4) {
			case 0:
				n.Say(script.NpcString(1000286, c.Name()))
			case 1:
				n.Say(script.NpcString(1000381, c.Name()))
			case 2:
				n.Say(script.NpcString(1000382))
			case 3:
				n.Say(script.NpcString(1000383))
			}
			defaultnpc.TryToAttack(s, n, c)
		}
	}
	return true
}

// seeSpell is WarriorBase's see-spell hook up to its parent call.
func seeSpell(s *script.Script, e script.SeeSpell) {
	n, caster := e.NPC, e.Caster
	mostHated := n.MostHated()
	intention := n.Intention()

	if caster.Level()+15 < n.Level() {
		n.RemoveAllAttackDesire()
		if script.Same(caster, mostHated) {
			if intention == script.IntentionAttack {
				if k := s.Skill(e.Skill); k.AggroPoints() > 0 || k.Power(n) > 0 || k.Offensive() {
					skillPower := max(max(float64(k.AggroPoints()), k.Power(n)), 20)
					hateRatio := defaultnpc.HateRatio(n, caster)
					hateRatio = ((1.0 * skillPower) / float64(n.Level()+7)) + ((hateRatio / 100) * ((1.0 * skillPower) / float64(n.Level()+7)))
					n.AddAttackDesire(caster, hateRatio*150)
				}
			}
			if n.HPRatio() != 1.0 && n.GeoPathFailCount() >= 10 {
				n.AbortAll()
				n.InstantTeleportTo(caster)
			}
		}
	}

	if mostHatedPlayer, ok := mostHated.(*script.Player); ok && script.ClassTypeOf(mostHatedPlayer.ClassID()) != player.ClassTypeMystic && intention == script.IntentionAttack {
		if swapPosition := n.IntParam("SwapPosition"); swapPosition != 0 && int32(s.Rnd(100)) < swapPosition {
			casterDistance := n.Distance2D(caster)
			if n.Distance2D(mostHatedPlayer) < casterDistance && casterDistance < 900 {
				casterLoc := script.LocOf(caster)
				mostHatedLoc := script.LocOf(mostHatedPlayer)
				caster.AbortAll()
				caster.InstantTeleportTo(mostHatedLoc)
				mostHatedPlayer.AbortAll()
				mostHatedPlayer.InstantTeleportTo(casterLoc)
				n.AddAttackDesire(caster, 1000)
			}
		}
	}
}
