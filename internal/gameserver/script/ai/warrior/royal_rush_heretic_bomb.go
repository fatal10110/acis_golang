package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// RoyalRushHereticBomb is RoyalRushHereticBomb: a RoyalRushBomb of the
// heretic rooms that keeps to its territory until it is drawn into a
// fight. Hit, or told of a hit on its clan, while neither attacking nor
// casting, it gives up the territory (i_ai4); out of the territory before
// that it drops its attack desires and walks back to its spawn. Script
// events 10002 and 10033 turn it on the playable they name (10033 also
// releases it from the territory), and 10035 sends it home bound to the
// territory again.
func RoyalRushHereticBomb() script.Script {
	return script.Script{Dir: royalRushBombDir, Behavior: true, NPCs: []int32{22145, 22154}, Hooks: royalRushHereticBombHooks()}
}

// royalRushBombDir is the page directory of the Four Sepulchers bombs.
const royalRushBombDir = "ai/individual/Monster/WarriorBase/Warrior/RoyalRushBomb"

// royalRushHereticBombHooks are RoyalRushHereticBomb's hooks, which its
// child derives its own from.
func royalRushHereticBombHooks() script.Hooks {
	base := royalRushBombHooks()
	goHome := func(n *script.NPC) {
		n.RemoveAllAttackDesire()
		n.AddMoveToDesire(n.SpawnLocation(), 100)
	}
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if !fighting(n.Intention()) && n.Int(script.SlotIntAI4) == 0 {
				n.SetInt(script.SlotIntAI4, 1)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if !fighting(called.Intention()) && called.Int(script.SlotIntAI4) == 0 {
				called.SetInt(script.SlotIntAI4, 1)
			}
			base.ClanAttacked(s, e)
		},
		OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
			if e.NPC.Int(script.SlotIntAI4) == 0 {
				goHome(e.NPC)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 10033, 10002:
				c0 := s.Object(e.Arg1)
				if c0 == nil {
					return
				}
				if e.EventID == 10033 {
					n.SetInt(script.SlotIntAI4, 1)
				}
				n.RemoveAllAttackDesire()
				if c0.Playable() {
					n.AddAttackDesire(c0, 200)
				}
			case 10035:
				n.SetInt(script.SlotIntAI4, 0)
				goHome(n)
			}
		},
	}).Named("RoyalRushHereticBomb")
}

// RoyalRushHereticBomb2 is RoyalRushHereticBomb2: a RoyalRushHereticBomb
// that, with nothing to do, walks back to its spawn instead of wandering.
func RoyalRushHereticBomb2() script.Script {
	return script.Script{Dir: royalRushBombDir, Behavior: true, NPCs: []int32{22195}, Hooks: royalRushHereticBombHooks().With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
	})}
}
