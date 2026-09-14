package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// FakeDeath reports false: the fake-death stance is not tracked as a state
// yet.
func (c *Character) FakeDeath() bool { return false }

// SilentMoving reports false: the silent-move stat is not modeled yet.
func (c *Character) SilentMoving() bool { return false }

// RaidRelated reports false: players are never raid bosses or minions.
func (c *Character) RaidRelated() bool { return false }

// Guard reports false: players are never town guards.
func (c *Character) Guard() bool { return false }

// Owner reports no owner: players are not summons.
func (c *Character) Owner() (attackable.Combatant, bool) { return nil, false }
