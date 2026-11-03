package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// FleeFromClanAggressive is WarriorFleeFromClanAggressive: a
// WarriorPhysicalSpecial that attacks the players and summons it sees and
// runs from a clan member that dies, as far as its Distance AI parameter.
func FleeFromClanAggressive() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21443, 21446, 21447, 21449}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			// A creature other than a player or summon reaches no parent.
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
		OnClanDied: func(s *script.Script, e script.ClanDied) {
			called := e.Called
			called.AddFleeDesire(e.Caller, int(called.IntParam("Distance")), 10000)
			base.ClanDied(s, e)
		},
	})}
}
