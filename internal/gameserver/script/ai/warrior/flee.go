package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Flee is WarriorFlee: a timid Warrior that wanders, and runs the
// configured drift range away from what hits it instead of fighting back.
func Flee() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{20002, 20432}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 20)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			e.NPC.AddFleeDesire(e.Attacker, e.NPC.MaxDriftRange(), 30)
		},
	})}
}
