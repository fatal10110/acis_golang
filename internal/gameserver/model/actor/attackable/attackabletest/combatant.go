// Package attackabletest provides a neutral attackable.Combatant for tests.
package attackabletest

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// Combatant supplies neutral values for every attackable.Combatant method
// except Position and Heading: an NPC that is alive, still, outside any peace
// zone, allowed to deal damage and with no owner. Embed it in a test double
// next to world.Presence (or define Position and Heading) and override only
// the methods the test exercises.
type Combatant struct{}

func (Combatant) ObjectID() int32                     { return 0 }
func (Combatant) Kind() actor.Kind                    { return actor.KindNPC }
func (Combatant) CollisionRadius() float64            { return 0 }
func (Combatant) CollisionHeight() float64            { return 0 }
func (Combatant) Level() int                          { return 0 }
func (Combatant) Karma() int                          { return 0 }
func (Combatant) Dead() bool                          { return false }
func (Combatant) AlikeDead() bool                     { return false }
func (Combatant) FakeDeath() bool                     { return false }
func (Combatant) RecentFakeDeath() bool               { return false }
func (Combatant) IsMoving() bool                      { return false }
func (Combatant) MovementDisabled() bool              { return false }
func (Combatant) InPeaceZone() bool                   { return false }
func (Combatant) SilentMoving() bool                  { return false }
func (Combatant) SpawnProtected() bool                { return false }
func (Combatant) CanGiveDamage() bool                 { return true }
func (Combatant) RaidRelated() bool                   { return false }
func (Combatant) SiegeGuard() bool                    { return false }
func (Combatant) Guard() bool                         { return false }
func (Combatant) Owner() (attackable.Combatant, bool) { return nil, false }
