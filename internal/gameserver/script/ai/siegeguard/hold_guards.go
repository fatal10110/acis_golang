package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// BowGuard is GludioBowGuard: a GludioHold archer that never runs from a
// higher attacker and helps a clan mate attacked from any height. It
// attacks a clan leader whose Seal of Ruler lands on it.
func BowGuard() script.Script {
	base := holdHooks()
	return script.Script{Dir: holdDir, Behavior: true, NPCs: bowGuardIDs(), Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if !spared(s, n, e.Attacker) && e.Attacker.Playable() {
				n.AddAttackDesire(e.Attacker, damageWeight(n, int(e.Damage), 100))
			}

			if n.InsideZone(script.ZonePeace) {
				n.TeleportTo(n.SpawnLocation())
				n.RemoveAllAttackDesire()
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if !spared(s, e.Called, e.Attacker) {
				e.Called.AddAttackDesire(e.Attacker, damageWeight(e.Called, int(e.Damage), 50))
			}
		},
		OnSpelled: func(_ *script.Script, e script.Spelled) {
			if e.Skill == sealOfRuler() {
				e.NPC.AddAttackDesire(e.Caster, 50000)
			}
		},
	})}
}

// bowGuardIDs are GludioBowGuard's own NPCs.
func bowGuardIDs() []int32 {
	return []int32{
		31401, 35017, 35027, 35037, 35047, 35057, 35072, 35075, 35078, 35114, 35117, 35120, 35156, 35159,
		35162, 35198, 35201, 35204, 35241, 35244, 35247, 35288, 35291, 35294, 35332, 35335, 35338, 35477,
		35480, 35483, 35524, 35527, 35530,
	}
}

// CourtGuard is GludioCourtGuard: a GludioHold guard that attacks a clan
// leader whose Seal of Ruler lands on it, its castle's own included.
func CourtGuard() script.Script {
	base := holdHooks()
	return script.Script{Dir: holdDir, Behavior: true, NPCs: courtGuardIDs(), Hooks: base.With(script.Hooks{
		OnSpelled: func(_ *script.Script, e script.Spelled) {
			if e.Skill == sealOfRuler() {
				e.NPC.AddAttackDesire(e.Caster, 50000)
			}
		},
	})}
}

// courtGuardIDs are GludioCourtGuard's own NPCs.
func courtGuardIDs() []int32 {
	return []int32{35069, 35111, 35153, 35195, 35238, 35285, 35329, 35474, 35521}
}

// DukeLewinWaldner is GludioDukeLewinWaldner: a GludioHold guard that
// attacks a clan leader whose Seal of Ruler lands on it, unless the leader
// defends or owns its castle.
func DukeLewinWaldner() script.Script {
	base := holdHooks()
	return script.Script{Dir: holdDir, Behavior: true, NPCs: dukeLewinWaldnerIDs(), Hooks: base.With(script.Hooks{
		OnSpelled: func(s *script.Script, e script.Spelled) {
			if e.Skill == sealOfRuler() && !spared(s, e.NPC, e.Caster) {
				e.NPC.AddAttackDesire(e.Caster, 50000)
			}
		},
	})}
}

// dukeLewinWaldnerIDs are GludioDukeLewinWaldner's own NPCs.
func dukeLewinWaldnerIDs() []int32 {
	return []int32{35064, 35106, 35148, 35190, 35234, 35280, 35325, 35470, 35517}
}

// Knight is GludioKnight: a GludioHold guard that attacks a clan leader
// whose Seal of Ruler lands on it, unless the leader defends or owns its
// castle.
func Knight() script.Script {
	base := holdHooks()
	return script.Script{Dir: holdDir, Behavior: true, NPCs: knightIDs(), Hooks: base.With(script.Hooks{
		OnSpelled: func(s *script.Script, e script.Spelled) {
			if e.Skill == sealOfRuler() && !spared(s, e.NPC, e.Caster) {
				e.NPC.AddAttackDesire(e.Caster, 50000)
			}
		},
	})}
}

// knightIDs are GludioKnight's own NPCs.
func knightIDs() []int32 {
	return []int32{35066, 35108, 35150, 35192, 35235, 35282, 35326, 35471, 35518}
}
