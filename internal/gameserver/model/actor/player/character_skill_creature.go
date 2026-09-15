package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// Players are never attackable NPCs and hold no aggro tables, so the NPC
// aggro controls below are no-ops.

func (c *Character) Attackable() bool                           { return false }
func (c *Character) NotifyAggression(attackable.Combatant, int) {}
func (c *Character) ReduceAllAggroHate(float64)                 {}
func (c *Character) StopAggroHate(attackable.Combatant)         {}
func (c *Character) StopHateList(attackable.Combatant)          {}
func (c *Character) ClearAggroTables()                          {}
