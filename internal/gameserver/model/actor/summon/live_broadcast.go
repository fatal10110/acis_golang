package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Runtime is what a summon needs to act in the live world beyond its
// construction config: the AI loop commands and effects drive, and the sink
// its events reach. Both are built from the actor, so they cannot be
// constructor config.
type Runtime struct {
	AI   AI
	Sink event.Sink
}

// Attach installs rt. Call it once, before SpawnBesideOwner publishes the
// summon into the world; a nil Sink drops every event so domain tests need no
// packet layer, and a nil AI leaves commands unexecuted.
func (a *Actor) Attach(rt Runtime) {
	a.brain = rt.AI
	a.sink = rt.Sink
}

func (a *Actor) emit(e event.Event) {
	if a.sink != nil {
		a.sink.Emit(e)
	}
}

// BroadcastAutoAttackStop reports that the owner's combat stance expired from
// inactivity.
func (a *Actor) BroadcastAutoAttackStop() {
	a.emit(event.AutoAttackStopped{})
}

func (a *Actor) BroadcastMove(ev event.Move) error {
	a.emit(ev)
	return nil
}

func (a *Actor) SyncPosition(position location.Location) {
	if a.world != nil {
		_ = a.world.Move(a, position.X, position.Y, position.Z)
	}
}

func (a *Actor) BroadcastStop() error {
	a.emit(event.Stopped{})
	return nil
}

func (a *Actor) SetHeadingTo(target attackable.Combatant) {
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return
	}
	sx, sy, _ := a.Position()
	tx, ty, _ := other.Position()
	a.Presence.SetHeading(location.Location{X: sx, Y: sy}.HeadingTo(location.Location{X: tx, Y: ty}))
}

func (a *Actor) BroadcastMoveToPawn(target attackable.Combatant) error {
	located, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return nil
	}
	sx, sy, sz := a.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	tx, ty, tz := located.Position()
	distance := int(origin.Distance3D(location.Location{X: tx, Y: ty, Z: tz}))
	a.emit(event.MoveToPawn{TargetID: target.ObjectID(), Distance: distance, Origin: origin})
	return nil
}

// BroadcastSelfSkillUse reports the cast-start animation of skillID at level
// with this summon as both caster and target, matching the reference's
// summon.broadcastPacket(new MagicSkillUse(summon, summon, ...)) self-cast
// shape.
func (a *Actor) BroadcastSelfSkillUse(skillID, level int32) error {
	x, y, z := a.Position()
	at := location.Location{X: x, Y: y, Z: z}
	a.emit(event.MagicSkillUse{CasterID: a.ObjectID(), CasterAt: at, TargetID: a.ObjectID(), TargetAt: at, SkillID: skillID, Level: level})
	return nil
}

// MarkDiscoveredByOwner records that the owner's client now knows this
// summon; abnormal-effect changes are reported only from then on.
func (a *Actor) MarkDiscoveredByOwner() { a.ownerDiscovered.Store(true) }

// StartAbnormalEffect adds mask to this summon's visible abnormal state.
func (a *Actor) StartAbnormalEffect(mask int) { a.abnormalEffect.Or(int32(mask)) }

// StopAbnormalEffect removes mask from this summon's visible abnormal state.
func (a *Actor) StopAbnormalEffect(mask int) {
	for {
		current := a.abnormalEffect.Load()
		if a.abnormalEffect.CompareAndSwap(current, current&^int32(mask)) {
			return
		}
	}
}

// AbnormalEffect returns this summon's visible abnormal-effect bitmask.
func (a *Actor) AbnormalEffect() int { return int(a.abnormalEffect.Load()) }

// UpdateAbnormalEffect reports the current state for re-announcement once the
// owner has discovered this summon.
func (a *Actor) UpdateAbnormalEffect() {
	if a.ownerDiscovered.Load() {
		a.emit(event.AbnormalEffectChanged{})
	}
}
