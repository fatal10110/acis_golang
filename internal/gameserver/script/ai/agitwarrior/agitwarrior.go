// Package agitwarrior holds the behaviors of the beasts and bandits that
// fight in the clan hall sieges of the Bandit Stronghold and the Wild
// Beast Reserve: AgitWarrior, which attacks whoever hits it and every
// player of another clan, and every attackable NPC, it sees, AgitWizard,
// derived from it, and AgitWarriorFlee, which breaks off and flees once
// worn down, with its AgitThief and AgitWizardFlee children.
package agitwarrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir and fleeDir are the page directories of the behaviors.
const (
	dir     = "ai/individual/AgitWarrior"
	fleeDir = "ai/individual/AgitWarrior/AgitWarriorFlee"
)

// Warrior is the AgitWarrior behavior.
func Warrior() script.Script {
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35428, 35618}, Hooks: WarriorHooks()}
}

// WarriorHooks are AgitWarrior's hooks, which its children derive theirs
// from. Hit by a player of another clan, or by a clanless one or its
// summon, the NPC wants to attack the attacker with its damage weight and,
// on a draw under 10 of 100, to strike it. It attacks, weight 200, every
// such player it sees and every attackable NPC; it looks around when a
// walk ends and wanders when it has nothing to do.
func WarriorHooks() script.Hooks {
	base := defaultnpc.New().Hooks
	return base.With(script.Hooks{
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			e.NPC.LookNeighbor()
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if p := script.ActingPlayer(e.Attacker); p != nil && outsider(e.NPC, p) {
				e.NPC.AddAttackDesire(e.Attacker, damageWeight(e))

				if s.Rnd(100) < 10 {
					e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4032, Level: 6}, 1000000)
				}
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if p := script.ActingPlayer(e.Creature); p != nil && outsider(e.NPC, p) {
				e.NPC.AddAttackDesire(p, 200)
			} else if seen, ok := e.Creature.(*script.NPC); ok && seen.Attackable() {
				e.NPC.AddAttackDesire(e.Creature, 200)
			}
		},
	}).Named("AgitWarrior")
}

// Wizard is AgitWizard: an AgitWarrior that, hit by a player of another
// clan, or by a clanless one or its summon, always wants to burn the
// attacker with its aura.
func Wizard() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35430, 35620}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if p := script.ActingPlayer(e.Attacker); p != nil && outsider(e.NPC, p) {
				e.NPC.AddAttackDesire(e.Attacker, damageWeight(e))

				if s.Rnd(100) < 100 {
					e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4077, Level: 6}, 1000000)
				}
			}
		},
	})}
}

// outsider reports whether p is of another clan than n stands for, or of
// none.
func outsider(n *script.NPC, p *script.Player) bool {
	return p.ClanID() != n.ClanID() || p.ClanID() == 0
}

// damageWeight is the attack desire weight of the hit e: 100 for each
// twentieth of the NPC's maximum hit points it took from a player, 10 from
// any other attacker.
func damageWeight(e script.Attacked) float64 {
	scale := 10.0
	if _, ok := e.Attacker.(*script.Player); ok {
		scale = 100
	}
	return float64(e.Damage) / float64(e.NPC.MaxHP()) / 0.05 * scale
}
