package summon

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// BroadcastFrame sends frame to every currently known observer capable of
// receiving one (i.e. a connected player session), from this summon's own
// known list. It takes ownership of frame and releases it. It is a no-op until
// SpawnBesideOwner has attached a world.
func (a *Actor) BroadcastFrame(frame wire.Frame) {
	defer frame.Release()
	if a.world == nil {
		return
	}
	a.world.ForEachKnown(a, func(o world.Tracked) {
		receiver, ok := o.(interface{ BroadcastFrame(wire.Frame) bool })
		if !ok {
			return
		}
		owned, ok := serverpackets.CopyFrame(frame)
		if ok {
			receiver.BroadcastFrame(owned)
		}
	})
}

func (a *Actor) BroadcastMove(event move.Event) error {
	a.BroadcastFrame(serverpackets.FrameMove(a.ObjectID(), event))
	return nil
}

func (a *Actor) SyncPosition(position location.Location) {
	if a.world != nil {
		_ = a.world.Move(a, position.X, position.Y, position.Z)
	}
}

func (a *Actor) BroadcastStop() error {
	x, y, z := a.Position()
	a.BroadcastFrame(serverpackets.FrameStopMove(a.ObjectID(), location.Location{X: x, Y: y, Z: z}, a.Heading()))
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
	a.BroadcastFrame(serverpackets.FrameMoveToPawn(a.ObjectID(), target.ObjectID(), distance, origin))
	return nil
}

// ForEachKnown visits the currently visible objects around this summon.
func (a *Actor) ForEachKnown(fn func(world.Tracked)) {
	if a.world != nil {
		a.world.ForEachKnown(a, fn)
	}
}

// SetAbnormalEffectUpdater installs the network-owned renderer for abnormal
// effect transitions after the owner discovers the summon.
func (a *Actor) SetAbnormalEffectUpdater(fn func()) {
	a.abnormalMu.Lock()
	defer a.abnormalMu.Unlock()
	a.onAbnormalUpdate = fn
}

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

// UpdateAbnormalEffect re-announces the current state to non-owner observers.
func (a *Actor) UpdateAbnormalEffect() {
	a.abnormalMu.RLock()
	fn := a.onAbnormalUpdate
	a.abnormalMu.RUnlock()
	if fn != nil {
		fn()
	}
}
