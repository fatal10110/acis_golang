package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const shotRangeDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorPhysicalSpecialShotRange"

// shootFromAfar makes n want its long range physical special skill on
// target.
func shootFromAfar(n *script.NPC, target script.Creature) {
	castByType(n, target, npc.SkillTypeLongRangePhysicalSpecial)
}

// PhysicalSpecialShotRange is WarriorPhysicalSpecialShotRange: a Warrior
// that, hit, wants its long range physical special skill on an attacker
// farther than 200, then, on a draw under 33 each, its physical special
// and its self range physical special skill on the attacker. Its 1002
// timer, every 5 seconds once started, shoots from afar at its last
// attacker while that one stands farther than 200, and forgets it
// otherwise. Its timer hook answers no other timer and calls no parent.
//
// Its memory: IntAI3 is its last attacker's object id.
func PhysicalSpecialShotRange() script.Script {
	return script.Script{Dir: shotRangeDir, Behavior: true, Hooks: shotRangeHooks()}
}

// shotRangeHooks are WarriorPhysicalSpecialShotRange's hooks, which its
// children derive theirs from.
func shotRangeHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)
			e.NPC.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetInt(script.SlotIntAI3, attacker.ObjectID())
			if n.Distance2D(attacker) > 200 {
				shootFromAfar(n, attacker)
			}
			if s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial)
			}
			if s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypeSelfRangePhysicalSpecial)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1002") {
				n := e.NPC
				if c0 := s.Object(n.Int(script.SlotIntAI3)); c0 != nil && n.Distance2D(c0) > 200 {
					shootFromAfar(n, c0)
				} else {
					n.SetInt(script.SlotIntAI3, 0)
				}
				s.StartTimer("1002", n, nil, 5000*time.Millisecond)
			}
			return ""
		},
	}).Named("WarriorPhysicalSpecialShotRange")
}

// PhysicalSpecialShotNoRange is WarriorPhysicalSpecialShotNoRange: a
// WarriorPhysicalSpecialShotRange whose created and dying hooks only pass
// on to its parent.
func PhysicalSpecialShotNoRange() script.Script {
	base := shotRangeHooks()
	return script.Script{Dir: shotRangeDir, Behavior: true, NPCs: []int32{22066, 22067, 22068, 22069, 22070, 22071, 22072, 22073}, Hooks: base.With(script.Hooks{
		// Both hooks only pass on to the parent; the areas they name are never switched.
		OnCreated: func(s *script.Script, e script.Created) { base.Created(s, e) },
		OnMyDying: func(s *script.Script, e script.MyDying) { base.MyDying(s, e) },
	})}
}

// PhysicalSpecialShotRangeAggressive is
// WarriorPhysicalSpecialShotRangeAggressive: a
// WarriorPhysicalSpecialShotRange that attacks the players and summons it
// sees, shoots from afar at one farther than 200 and starts its 1002 timer.
func PhysicalSpecialShotRangeAggressive() script.Script {
	return script.Script{Dir: shotRangeDir, Behavior: true, NPCs: []int32{22053}, Hooks: shotRangeAggressiveHooks()}
}

// shotRangeAggressiveHooks are WarriorPhysicalSpecialShotRangeAggressive's
// hooks, which its children derive theirs from.
func shotRangeAggressiveHooks() script.Hooks {
	base := shotRangeHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			if n.Distance2D(c) > 200 {
				shootFromAfar(n, c)
			}
			s.StartTimer("1002", n, nil, 5000*time.Millisecond)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorPhysicalSpecialShotRangeAggressive")
}

// PhysicalSpecialShotRangeDebuffAggressive is
// WarriorPhysicalSpecialShotRangeDebuffAggressive: a
// WarriorPhysicalSpecialShotRangeAggressive that, hit below a third of its
// HP, wants its first debuff on the attacker on a draw under 60, and
// otherwise one of its other four, two draws under 50 choosing which.
func PhysicalSpecialShotRangeDebuffAggressive() script.Script {
	base := shotRangeAggressiveHooks()
	return script.Script{Dir: shotRangeDir, Behavior: true, NPCs: []int32{22054}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.HPRatio() < 0.33 {
				switch {
				case s.Rnd(100) < 60:
					castByType(n, attacker, npc.SkillTypeDebuff1)
				case s.Rnd(100) < 50:
					if s.Rnd(100) < 50 {
						castByType(n, attacker, npc.SkillTypeDebuff2)
					} else {
						castByType(n, attacker, npc.SkillTypeDebuff3)
					}
				case s.Rnd(100) < 50:
					castByType(n, attacker, npc.SkillTypeDebuff4)
				default:
					castByType(n, attacker, npc.SkillTypeDebuff5)
				}
			}
			base.Attacked(s, e)
		},
	})}
}
