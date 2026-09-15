package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// Summons are never attackable NPCs and hold no aggro tables, so the NPC
// aggro controls below are no-ops.

func (a *Actor) Attackable() bool                           { return false }
func (a *Actor) NotifyAggression(attackable.Combatant, int) {}
func (a *Actor) ReduceAllAggroHate(float64)                 {}
func (a *Actor) StopAggroHate(attackable.Combatant)         {}
func (a *Actor) StopHateList(attackable.Combatant)          {}
func (a *Actor) ClearAggroTables()                          {}
func (a *Actor) EnableOverhit()                             {}
