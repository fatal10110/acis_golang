package guard

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// MoveAround is GuardMoveAround: a Guard that, with nothing to do, wanders
// around its spawn point.
func MoveAround() script.Script {
	return script.Script{Dir: dir, Behavior: true, NPCs: moveAroundIDs(), Hooks: MoveAroundHooks()}
}

// moveAroundIDs are GuardMoveAround's own NPCs.
func moveAroundIDs() []int32 {
	return []int32{31844, 31845, 31846, 31847, 31848, 31849, 31850, 31851, 31853}
}

// MoveAroundHooks are GuardMoveAround's hooks, which its children derive
// theirs from.
func MoveAroundHooks() script.Hooks {
	return GuardHooks().With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
	}).Named("GuardMoveAround")
}
