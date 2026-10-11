package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// Hold is WarriorHold: a Warrior that never wanders. With nothing to do it
// walks back to its spawn point, or stands still once it is there.
func Hold() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorHold", Behavior: true, NPCs: holdIDs(), Hooks: HoldHooks()}
}

// holdIDs are WarriorHold's own NPCs.
func holdIDs() []int32 {
	return []int32{27109, 27105, 27102, 27139, 27104, 27107, 27106, 27103}
}

// HoldHooks are WarriorHold's hooks, which its children derive theirs from.
func HoldHooks() script.Hooks {
	return WarriorHooks().With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.Distance2D(n.SpawnLocation()) > 0 {
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			} else {
				n.AddDoNothingDesire(40, 30)
			}
		},
	})
}
