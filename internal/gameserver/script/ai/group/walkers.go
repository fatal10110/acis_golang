// Package group holds the NPC behaviors shared by groups of NPCs.
package group

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// walkerWeight is the weight of a walker's route desire: nothing a walker
// otherwise wants outweighs its walk.
const walkerWeight = 1000000

// Walkers sends the town walkers along their routes from the moment they
// are created, each on the route listed under its template alias; the ones
// in walkingNPCs walk it, the rest run it.
func Walkers() script.Script {
	return script.Script{
		Dir: "ai/group/Walker",
		Bind: script.Bindings{script.EventCreated: {
			31356, 31357, 31358, 31359, 31360, 31361, 31362, 31363, 31364, 31365, 31525, 31705, 32070, 32072, 32128,
		}},
		Hooks: script.Hooks{OnCreated: onCreated},
	}
}

// walkingNPCs are the walkers that walk their route rather than run it.
func walkingNPCs() []int32 {
	return []int32{31357, 31358, 31359, 31360, 31362, 31364, 31365, 31525, 32072, 32128}
}

// onCreated puts a walker in its walk stance when it is one of walkingNPCs,
// then asks it to walk its route.
func onCreated(_ *script.Script, e script.Created) {
	if slices.Contains(walkingNPCs(), e.NPC.NpcID()) {
		e.NPC.SetRunning(false)
	}
	e.NPC.AddMoveRouteDesire(e.NPC.Alias(), walkerWeight)
}
