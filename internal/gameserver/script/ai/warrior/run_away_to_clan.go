package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const runAwayToClanDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorRunAwayToClan"

// RunAwayToClan is WarriorRunAwayToClan: a Warrior that lets its first hit
// pass and, on a later one with a draw under 10, between a third and half
// its HP and an attacker above a quarter of its HP, shouts one of its
// flee lines and runs to the flee point its spawn names. There it wanders,
// calls its kin within 400 to the creature that drove it off (script event
// 10000) and, 15 seconds later while it does not attack, walks home: in
// earnest below 70% HP, else idly with its desires dropped. It ignores the
// creatures it sees. Told of event 10000 while alive and not attacking, it
// attacks the creature named.
//
// Its memory: IntAI0 is 0 idle, 1 running, 2 at the flee point and 3 on
// its way home; IntAI4 is set once it was hit; Creature0 is the creature
// it fled.
func RunAwayToClan() script.Script {
	return script.Script{Dir: runAwayToClanDir, Behavior: true, NPCs: []int32{20058, 20207, 20439, 20208, 20437, 20210, 20436, 20499, 20500, 20498, 20494}, Hooks: runAwayToClanHooks()}
}

// runAwayToClanHooks are WarriorRunAwayToClan's hooks, which its children
// derive theirs from.
func runAwayToClanHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 {
				n.SetInt(script.SlotIntAI4, 1)
			} else if fleeLoc, ok := n.FleeLocation(); ok && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 10 {
				npcHP, attackerHP := n.HPRatio(), e.Attacker.HPRatio()
				if npcHP < 0.5 && npcHP > 0.33 && attackerHP > 0.25 {
					n.Say(script.NpcString(fleeLine(s.Rnd(100))))
					n.AddMoveToDesire(fleeLoc, 100000000)
					n.SetInt(script.SlotIntAI0, 1)
					n.SetSlot(script.SlotCreature0, e.Attacker)
				}
			}
			base.Attacked(s, e)
		},
		// It ignores what it sees, its parents' look included.
		OnSeeCreature: func(*script.Script, script.SeeCreature) {},
		OnMoveToFinished: func(s *script.Script, e script.MoveToFinished) {
			n := e.NPC
			switch n.Int(script.SlotIntAI0) {
			case 1:
				fleeLoc, ok := n.FleeLocation()
				if !ok {
					return
				}
				if !arrivedAt(fleeLoc, e) {
					n.AddMoveToDesire(fleeLoc, 100000000)
					return
				}
				s.StartTimer("2001", n, nil, 15000*time.Millisecond)
				// The finished walk is the current desire, dropped once the
				// hooks return; no other walk is matched here.
				n.AddWanderDesire(5, 50)
				n.SetInt(script.SlotIntAI0, 2)
				if c0 := n.Slot(script.SlotCreature0); c0 != nil {
					s.BroadcastScriptEvent(n, 10000, c0.ObjectID(), 400)
				}
			case 3:
				if arrivedAt(n.SpawnLocation(), e) {
					n.AddWanderDesire(5, 1000)
					n.SetInt(script.SlotIntAI0, 0)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") && n.Intention() != script.IntentionAttack {
				if n.HPRatio() < 0.7 && n.Int(script.SlotIntAI0) == 2 {
					n.RemoveDesires(script.IntentionWander)
					n.AddMoveToDesire(n.SpawnLocation(), 1000000)
					n.SetInt(script.SlotIntAI0, 3)
				} else {
					n.RemoveAllDesire()
					n.AddMoveToDesire(n.SpawnLocation(), 50)
					n.SetInt(script.SlotIntAI0, 0)
				}
			}
			return base.Timer(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.Dead() || e.EventID != 10000 || n.Intention() == script.IntentionAttack {
				return
			}
			if c := s.Object(e.Arg1); c != nil {
				n.AddAttackDesire(c, 1000000)
			}
			n.SetInt(script.SlotIntAI3, 0)
		},
	}).Named("WarriorRunAwayToClan")
}

// fleeLine returns the NpcString a fleeing runner shouts on draw i5 of 100:
// each of the first eleven lines on 7 draws, each of the next nine on 2,
// the last on the rest.
func fleeLine(i5 int) int32 {
	id := int32(1000007)
	for _, bound := range [...]int{7, 14, 21, 28, 35, 42, 49, 56, 63, 70, 77, 79, 81, 83, 85, 87, 89, 91, 93, 95} {
		if i5 < bound {
			return id
		}
		id++
	}
	return id
}

// RunAwayToClanAggressive is WarriorRunAwayToClanAggressive: a
// WarriorRunAwayToClan that attacks the players and summons it sees.
func RunAwayToClanAggressive() script.Script {
	base := runAwayToClanHooks()
	return script.Script{Dir: runAwayToClanDir, Behavior: true, NPCs: []int32{20211, 20438, 20061, 20497, 20495}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}
