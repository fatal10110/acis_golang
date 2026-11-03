package siegeguard

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Hold is GludioHold. It binds no NPC of its own.
func Hold() script.Script {
	return script.Script{Dir: holdDir, Behavior: true, Hooks: holdHooks()}
}

// holdHooks are GludioHold's hooks, which the guards derived from it
// derive theirs from. The guard attacks the players and summons that hit
// it, hit a clan mate, hit its castle's doors or come into its sight, and
// one casting at what it fights; it spares its castle's defenders, and
// runs from an attacker standing more than 100 above it. Standing in a
// peace zone, it is sent back to its post, dropping its attacks; with
// nothing to do it walks back there and keeps still.
func holdHooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if above(e.Attacker, n) {
				n.AddFleeDesire(e.Attacker, 150, 30)
			} else if !spared(s, n, e.Attacker) && e.Attacker.Playable() {
				n.AddAttackDesire(e.Attacker, damageWeight(n, int(e.Damage), 100))
			}

			if n.InsideZone(script.ZonePeace) {
				n.TeleportTo(n.SpawnLocation())
				n.RemoveAllAttackDesire()
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if !above(e.Attacker, e.Called) && !spared(s, e.Called, e.Attacker) {
				e.Called.AddAttackDesire(e.Attacker, damageWeight(e.Called, int(e.Damage), 50))
			}
		},
		OnStaticObjectClanAttacked: func(s *script.Script, e script.StaticObjectClanAttacked) {
			switch {
			case e.Attacker.SiegeSummon():
				if s.Rnd(100) < 10 {
					e.Called.AddAttackDesire(script.ActingPlayer(e.Attacker), 5000)
				} else {
					e.Called.AddAttackDesire(e.Attacker, 1000)
				}
			case !spared(s, e.Called, e.Attacker) && e.Attacker.Playable():
				e.Called.AddAttackDesire(e.Attacker, damageWeight(e.Called, int(e.Damage), 50))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			if above(e.Caster, n) || spared(s, n, e.Caster) {
				return
			}
			k := s.Skill(e.Skill)
			if k.Offensive() && n.InCombat() && targets(e.Targets, n.TopDesireTarget()) {
				n.AddAttackDesire(e.Caster, damageWeight(n, k.AggroPoints(), 50))

				if n.GeoPathFailCount() > 10 && n.HPRatio() < 1.0 {
					n.TeleportTo(e.Caster)
				}
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if !above(e.Creature, n) && e.Creature.Playable() && !spared(s, n, e.Creature) && n.LifeTime() > 7 {
				n.AddAttackDesire(e.Creature, 200)
			}

			if n.InsideZone(script.ZonePeace) {
				n.TeleportTo(n.SpawnLocation())
				n.RemoveAllAttackDesire()
			}
		},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			n := e.NPC
			if home := n.SpawnLocation(); home.X != int(e.X) || home.Y != int(e.Y) || home.Z != int(e.Z) {
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			} else {
				n.AddDoNothingDesire(40, 30)
			}
		},
	}).Named("GludioHold")
}

// targets reports whether c is among a spell's targets; no creature is
// among none.
func targets(of []script.Creature, c script.Creature) bool {
	return c != nil && slices.ContainsFunc(of, func(t script.Creature) bool { return script.Same(t, c) })
}
