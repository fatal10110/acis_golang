package guard

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// Stand is GuardStand: a Guard that keeps its post. With nothing to do it
// walks back to its spawn point, and once there it stands still.
func Stand() script.Script {
	return script.Script{Dir: dir, Behavior: true, NPCs: standIDs(), Hooks: StandHooks()}
}

// standIDs are GuardStand's own NPCs.
func standIDs() []int32 {
	return []int32{
		30042, 30043, 30046, 30074, 30076, 30124, 30125, 30126, 30128, 30198, 30216, 30219, 30221, 30285,
		30333, 30337, 30347, 30348, 30357, 30382, 30384, 30386, 30430, 30431, 30432, 30433, 30452, 30466,
		30543, 30544, 30545, 30548, 30580, 30582, 30583, 30584, 30708, 30710, 30711, 30712, 30714, 30724,
		30725, 30726, 30872, 30874, 30876, 30882, 30884, 30886, 30888, 30918, 30920, 30922, 31293, 31294,
		31295, 31296, 31297, 31298, 31299, 31341, 31342, 31343, 31344, 31345, 31346, 31347, 31677, 31678,
		31681, 31682, 31985, 31988, 30039, 30071, 30121, 30196, 30224, 30284, 30332, 30707, 30723, 30870,
		30880, 30916, 31292, 31340,
	}
}

// StandHooks are GuardStand's hooks, which its children derive theirs
// from.
func StandHooks() script.Hooks {
	return GuardHooks().With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			n := e.NPC
			if home := n.SpawnLocation(); home.X != int(e.X) || home.Y != int(e.Y) || home.Z != int(e.Z) {
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			} else {
				n.AddDoNothingDesire(40, 30)
			}
		},
	}).Named("GuardStand")
}
