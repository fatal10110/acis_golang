package warrior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// AggressiveImmediate is WarriorAggressiveImmediate: a Warrior that wants
// to attack, with weight 200, every player or summon it sees inside its
// territory, whatever its aggressive time.
func AggressiveImmediate() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: aggressiveImmediateIDs(), Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			if e.NPC.InMyTerritory() {
				e.NPC.AddAttackDesire(e.Creature, 200)
			}
			base.SeeCreature(s, e)
		},
	})}
}

// aggressiveImmediateIDs are WarriorAggressiveImmediate's own NPCs.
func aggressiveImmediateIDs() []int32 {
	return []int32{
		27178, 20780, 27151, 27056, 27082, 27160, 27138, 27149, 27135, 27172, 27088, 27162, 27168, 27169,
		27170, 27164, 27044, 27045, 27157, 27083, 27152, 27092, 27080, 27034, 27033, 27032, 27089, 27136,
		27161, 27144, 27148, 27145, 27147, 27146, 27035, 27134, 27159, 27158, 27163, 27043, 27081, 27143,
		27094, 27137, 27150, 27180, 27194,
	}
}
