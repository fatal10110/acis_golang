package monster

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// LV3PartyLeader is LV3PartyLeaderMonster: an LV3Monster that creates its
// privates as it spawns. A hit on its party by any player but the one its
// second spawn parameter names makes it tell its master (event 1000) and
// leave the world; that player's playable hits on the party add to its
// attack desire. It binds no NPC of its own.
func LV3PartyLeader() script.Script {
	return script.Script{Dir: lv3Dir, Behavior: true, Hooks: lv3PartyLeaderHooks()}
}

// lv3PartyLeaderHooks are LV3PartyLeaderMonster's hooks, which its
// children derive theirs from.
func lv3PartyLeaderHooks() script.Hooks {
	base := lv3MonsterHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			defaultnpc.CreatePrivates(s, e.NPC)
			base.Created(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if c0 := script.ActingPlayer(target); c0 != nil {
				if c0.ObjectID() != called.Int(script.SlotParam2) {
					if c1 := called.Slot(script.SlotCreature1); c1 != nil {
						s.SendScriptEvent(c1.(*script.NPC), 1000, 0, 0)
					}
					called.DeleteMe()
				} else if target.Playable() {
					// Both operands are integers: the share truncates.
					called.AddAttackDesire(target, (float64(e.Damage/called.MaxHP())/0.05)*50)
				}
			}
		},
	}).Named("LV3PartyLeaderMonster")
}

// LV3Dwarf is an LV3PartyLeaderMonster that also answers a playable's hit
// on another member of its party with the party formula, weighted by that
// member's weight point, and replaces each private that dies with a new
// one of its kind.
func LV3Dwarf() script.Script {
	base := lv3PartyLeaderHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27259, 27260, 27290}, Hooks: base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if !script.Same(caller, called) && target.Playable() {
				f0 := defaultnpc.HateRatio(called, target)
				f0 = ((1.0 * float64(e.Damage)) / float64(called.Level()+7)) + ((f0 / 100) * ((1.0 * float64(e.Damage)) / float64(called.Level()+7)))
				called.AddAttackDesire(target, ((f0*float64(e.Damage))*float64(caller.Int(script.SlotWeightPoint)))*10)
			}
			base.PartyAttacked(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(caller, called) {
				s.Spawner().CreateOnePrivate(called, caller.NpcID(), 0)
			}
		},
	})}
}

// LV3Healer is an LV3PartyLeaderMonster that also answers a playable's hit
// on another member of its party with the party formula, weighted by that
// member's weight point, and replaces each private that dies with a new
// one of its kind.
func LV3Healer() script.Script {
	base := lv3PartyLeaderHooks()
	return script.Script{Dir: lv3Dir, Behavior: true, NPCs: []int32{27266, 27267}, Hooks: base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if !script.Same(caller, called) && target.Playable() {
				f0 := defaultnpc.HateRatio(called, target)
				f0 = ((1.0 * float64(e.Damage)) / float64(called.Level()+7)) + ((f0 / 100) * ((1.0 * float64(e.Damage)) / float64(called.Level()+7)))
				called.AddAttackDesire(target, ((f0*float64(e.Damage))*float64(caller.Int(script.SlotWeightPoint)))*10)
			}
			base.PartyAttacked(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(caller, called) {
				s.Spawner().CreateOnePrivate(called, caller.NpcID(), 0)
			}
		},
	})}
}
