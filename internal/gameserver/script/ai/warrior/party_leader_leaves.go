package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeaderBombSpawn is PartyLeaderBombSpawn: a PartyLeaderWarrior that
// spawns quietly, with no privates and no return timer, at weight point
// 10. The first player or summon it sees makes it create its privates
// (unless it summons them on a hit instead) and attack. Told by event
// 10000 of a creature while not attacking, it wants to attack it.
func PartyLeaderBombSpawn() script.Script {
	base := PartyLeaderHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{21517, 21512}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotWeightPoint, 10)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if c.Playable() && n.Int(script.SlotIntAI0) == 0 {
				n.SetInt(script.SlotIntAI0, 1)
				if n.IntParam("SummonPrivateRate") == 0 {
					defaultnpc.CreatePrivates(s, n)
				}
				defaultnpc.TryToAttack(s, n, c)
			}
			base.SeeCreature(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.Dead() {
				return
			}
			if e.EventID == 10000 {
				if n.Intention() != script.IntentionAttack {
					if c0 := s.Object(e.Arg1); c0 != nil {
						n.AddAttackDesire(c0, 1000)
					}
				}
			}
			base.ScriptEvent(s, e)
		},
	})}
}

// PartyLeaderPatrol is PartyLeaderPatrol: a PartyLeaderWarrior that never
// wanders.
func PartyLeaderPatrol() script.Script {
	base := PartyLeaderHooks()
	return script.Script{Dir: partyLeaderDir, Behavior: true, NPCs: []int32{27041}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
	})}
}
