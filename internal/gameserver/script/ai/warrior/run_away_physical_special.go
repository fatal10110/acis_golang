package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const runAwayPhysicalSpecialDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorRunAwayPhysicalSpecial"

// RunAwayPhysicalSpecial is WarriorRunAwayPhysicalSpecial: a Warrior that,
// hit below half its HP, may run, on a draw under 20, to the flee point
// its spawn names. There it wanders and, a second later, either turns on
// the creature it last fought and calls its kin within 400 to it (script
// event 10000), or waits to heal fully and walks home. Held by a root's
// kind of effect farther than 40 from the creature it hates most, it gives
// up a run, stops hating that one when it cannot attack it, and wants to
// attack a player or summon hitting it. On a draw under 33 it strikes with
// its physical special skill the player or summon it hates most when hit
// by that one. Told of event 10000 while it does not attack, it attacks
// the creature named.
//
// Its memory: IntAI0 is 0 idle, 1 running, 2 at the flee point and 3 back
// on its way; Creature0 is the creature it last fought.
func RunAwayPhysicalSpecial() script.Script {
	return script.Script{Dir: runAwayPhysicalSpecialDir, Behavior: true, NPCs: []int32{21508, 21510, 21511, 21513, 21515, 21516}, Hooks: runAwayPhysicalSpecialHooks()}
}

// RunAwayPhysicalSpecialAggressive is
// WarriorRunAwayPhysicalSpecialAggressive: a WarriorRunAwayPhysicalSpecial
// that attacks the players and summons it sees, its parents' look left
// out.
func RunAwayPhysicalSpecialAggressive() script.Script {
	base := runAwayPhysicalSpecialHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorRunAwayPhysicalSpecialAggressive", Behavior: true, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if e.Creature.Playable() {
				defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			}
		},
	})}
}

// runAwayPhysicalSpecialHooks are WarriorRunAwayPhysicalSpecial's hooks,
// which its children derive theirs from.
func runAwayPhysicalSpecialHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			mostHated := n.MostHated()
			if mostHated != nil {
				n.SetSlot(script.SlotCreature0, mostHated)
				if s.AbnormalLevel(n, skill.Ref{ID: 1201, Level: 1}) >= 0 && n.Distance2D(mostHated) > 40 {
					if n.Int(script.SlotIntAI0) == 1 {
						n.SetInt(script.SlotIntAI0, 3)
						n.RemoveAllDesire()
					}
					if !n.CanAttack(mostHated) {
						n.StopHate(mostHated)
					}
					if attacker.Playable() {
						// The parent hears the raised damage too.
						if e.Damage == 0 {
							e.Damage = 1
						}
						n.AddAttackDesire(attacker, float64(e.Damage)/float64(n.Level()+7)*100)
					}
				}
			} else {
				n.SetSlot(script.SlotCreature0, attacker)
			}
			if fleeLoc, ok := n.FleeLocation(); ok && n.HPRatio() < 0.5 && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 20 {
				n.SetInt(script.SlotIntAI0, 1)
				n.RemoveAllDesire()
				n.AddMoveToDesire(fleeLoc, 2000)
				return
			}
			if attacker.Playable() && script.Same(mostHated, attacker) && s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypePhysicalSpecial)
			}
			base.Attacked(s, e)
		},
		OnMoveToFinished: func(s *script.Script, e script.MoveToFinished) {
			n := e.NPC
			switch n.Int(script.SlotIntAI0) {
			case 1:
				if fleeLoc, ok := n.FleeLocation(); ok && arrivedAt(fleeLoc, e) {
					n.RemoveAllDesire()
					n.AddWanderDesire(5, 50)
					n.SetInt(script.SlotIntAI0, 2)
					name := "2002"
					if s.Rnd(100) < 50 {
						name = "2001"
					}
					s.StartTimer(name, n, nil, 1000*time.Millisecond)
				}
			case 3:
				if arrivedAt(n.SpawnLocation(), e) {
					n.SetInt(script.SlotIntAI0, 0)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "2001"):
				if n.Int(script.SlotIntAI0) == 2 {
					if c0 := n.Slot(script.SlotCreature0); c0 != nil {
						n.AddAttackDesire(c0, 1000)
						s.BroadcastScriptEvent(n, 10000, c0.ObjectID(), 400)
					}
					n.SetInt(script.SlotIntAI0, 3)
				}
			case strings.EqualFold(e.Name, "2002"):
				if n.Int(script.SlotIntAI0) == 2 {
					if n.HPRatio() == 1.0 {
						n.AddMoveToDesire(n.SpawnLocation(), 50000)
					} else {
						s.StartTimer("2002", n, nil, 1000*time.Millisecond)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID != 10000 || e.NPC.Intention() == script.IntentionAttack {
				return
			}
			if c0 := s.Object(e.Arg1); c0 != nil {
				e.NPC.AddAttackDesire(c0, 1000)
			}
		},
	}).Named("WarriorRunAwayPhysicalSpecial")
}

// arrivedAt reports whether the walk e ended exactly at l.
func arrivedAt(l script.Loc, e script.MoveToFinished) bool {
	return l.X == int(e.X) && l.Y == int(e.Y) && l.Z == int(e.Z)
}
