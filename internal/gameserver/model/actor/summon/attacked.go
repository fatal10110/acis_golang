package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	// avoidRadius is how far from its owner a summon steps aside when a hit
	// reaches it; the owner has to be within twice that of the summon.
	avoidRadius = 70
	// avoidPoints is how many evenly spaced spots around the owner a summon
	// picks from when it steps aside.
	avoidPoints = 12
)

// NotifyAttacked reports a damaging physical hit, or an offensive skill,
// from attacker reaching the summon: its owner enters attack stance, then
// the summon steps aside as AvoidAttack describes.
func (a *Actor) NotifyAttacked(attacker attackable.Combatant) {
	a.emit(event.Attacked{Attacker: attacker})
	a.AvoidAttack(attacker)
}

// NotifyEvaded reports a physical hit from attacker that missed the summon,
// which steps aside as AvoidAttack describes.
func (a *Actor) NotifyEvaded(attacker attackable.Combatant) {
	a.AvoidAttack(attacker)
}

// AvoidAttack moves an idle or following summon out of attacker's way, to a
// random one of twelve spots around its owner. It stays put when the owner is
// the attacker, is not within twice avoidRadius of it or is out of attack
// stance, or when the summon is moving, dead or unable to move.
func (a *Actor) AvoidAttack(attacker attackable.Combatant) {
	owner := a.owner
	if owner == nil || a.brain == nil {
		return
	}
	if attacker != nil && attacker.ObjectID() == owner.ObjectID() {
		return
	}
	ox, oy, oz := owner.Position()
	sx, sy, sz := a.Position()
	center := location.Location{X: ox, Y: oy, Z: oz}
	if !center.In3DRadius(location.Location{X: sx, Y: sy, Z: sz}, 2*avoidRadius) || !owner.InCombat() {
		return
	}
	if a.IsMoving() || a.Dead() || a.MovementDisabled() {
		return
	}
	a.brain.StepAside(location.EquidistantPoint(ox, oy, oz, avoidRadius, avoidPoints, a.Roll(avoidPoints)))
}
