package guard

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// Fixed is GuardFixed: a GuardStand that fights from where it stands. It
// wants to attack whoever hits it and the karma players it sees, but only
// while they are in its reach.
func Fixed() script.Script {
	return script.Script{Dir: dir, Behavior: true, NPCs: fixedIDs(), Hooks: StandHooks().With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			e.NPC.AddAttackDesireHold(e.Attacker, 2000)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if _, ok := e.Creature.(*script.Player); ok && script.ActingPlayer(e.Creature).Karma() > 0 {
				e.NPC.AddAttackDesireHold(e.Creature, 1500)
			}
		},
	})}
}

// fixedIDs are GuardFixed's own NPCs.
func fixedIDs() []int32 {
	return []int32{
		30040, 30041, 30044, 30045, 30072, 30073, 30075, 30122, 30123, 30197, 30199, 30200, 30201, 30217,
		30218, 30220, 30331, 30334, 30335, 30336, 30338, 30346, 30349, 30355, 30356, 30379, 30380, 30381,
		30383, 30385, 30465, 30478, 30541, 30542, 30546, 30547, 30577, 30578, 30579, 30581, 30709, 30713,
		30733, 30871, 30873, 30875, 30877, 30883, 30885, 30887, 30889, 30917, 30919, 30921, 30923, 31671,
		31672, 31673, 31674,
	}
}
