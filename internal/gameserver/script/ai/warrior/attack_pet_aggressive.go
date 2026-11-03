package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// eventAttackPet names, in Arg1, a creature a WarriorAttackPetAggressive
// is told to attack.
const eventAttackPet = 10017

// AttackPetAggressive is WarriorAttackPetAggressive: a WarriorAggressive
// that turns on the creature a script event names.
func AttackPetAggressive() script.Script {
	base := AggressiveHooks()
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: []int32{21444, 21445, 21448}, Hooks: base.With(script.Hooks{
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			if e.EventID == eventAttackPet {
				if c0 := s.Object(e.Arg1); c0 != nil {
					e.NPC.AddAttackDesire(c0, 100)
				}
			}
			base.ScriptEvent(s, e)
		},
	})}
}
