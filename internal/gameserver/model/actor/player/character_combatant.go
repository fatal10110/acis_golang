package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// FakeDeath reports whether an active effect makes this player play dead.
func (c *Character) FakeDeath() bool { return c.FakeDead() }

// RaidRelated reports false: players are never raid bosses or minions.
func (c *Character) RaidRelated() bool { return false }

// Guard reports false: players are never town guards.
func (c *Character) Guard() bool { return false }

// Owner reports no owner: players are not summons.
func (c *Character) Owner() (attackable.Combatant, bool) { return nil, false }

// RaceMultiplier reports 1: only NPC races scale damage.
func (c *Character) RaceMultiplier(creature.FormulaActor) float64 { return 1 }

// OwnsOffensiveFollowTicker reports false: a player's offensive follow is
// rechecked by its move controller.
func (c *Character) OwnsOffensiveFollowTicker() bool { return false }
