package summon

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// Karma reports 0: a summon carries no karma of its own.
func (a *Actor) Karma() int { return 0 }

// FakeDeath reports false: summons never feign death.
func (a *Actor) FakeDeath() bool { return false }

// RecentFakeDeath reports false: summons never feign death.
func (a *Actor) RecentFakeDeath() bool { return false }

// SilentMoving reports false: summons never move silently.
func (a *Actor) SilentMoving() bool { return false }

// SpawnProtected reports false: spawn protection is a player state.
func (a *Actor) SpawnProtected() bool { return false }

// RaidRelated reports false: summons are never raid bosses or minions.
func (a *Actor) RaidRelated() bool { return false }

// Guard reports false: summons are never town guards.
func (a *Actor) Guard() bool { return false }

// Owner returns the summon's controlling player.
func (a *Actor) Owner() (attackable.Combatant, bool) {
	if a.owner == nil {
		return nil, false
	}
	return a.owner, true
}

// CanSeeTarget reports true: the launch-phase line-of-sight gate is not wired
// for summon casts yet, so it never aborts one.
func (a *Actor) CanSeeTarget(skilltarget.Creature) bool { return true }
