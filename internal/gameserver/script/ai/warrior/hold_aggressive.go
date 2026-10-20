package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// HoldAggressive is WarriorHoldAggressive: a WarriorAggressive that, with
// no desire left, walks back to where it spawned instead of wandering.
func HoldAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{21661, 21684, 21707, 21730, 21753, 21776}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.Distance2D(n.SpawnLocation()) > 0 {
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			}
		},
	})}
}
