package summon

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// FrameBuilder translates this summon's broadcast-worthy state changes into
// wire frames. The network layer implements it (see
// serverpackets.NpcFrameBuilder) so this package never constructs packets or
// touches wire encoding itself — it only knows *when* to broadcast and *who*
// is listening, mirroring npc.FrameBuilder.
type FrameBuilder interface {
	Attack(snapshot attack.Snapshot) wire.Frame
	Move(objectID int32, event move.Event) wire.Frame
	MoveToPawn(objectID, targetID int32, distance int, origin location.Location) wire.Frame
	Stop(objectID int32, at location.Location, heading int) wire.Frame
	SkillUse(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int, success bool) wire.Frame
}

// SetFrameBuilder records the network-owned hook that translates this
// summon's broadcasts into packets. A nil builder keeps every Broadcast*
// method a silent no-op so domain tests need no packet layer.
func (a *Actor) SetFrameBuilder(b FrameBuilder) { a.frames = b }

// SetAutoAttackStopBroadcaster records the packet-layer hook that broadcasts
// AutoAttackStop to this summon's known observers when its owner's combat
// stance expires. A nil hook keeps BroadcastAutoAttackStop a silent no-op.
func (a *Actor) SetAutoAttackStopBroadcaster(fn func()) {
	a.broadcastAutoAttackStop = fn
}

// BroadcastAutoAttackStop sends AutoAttackStop through the runtime packet
// hook when the owner's combat stance expires from inactivity.
func (a *Actor) BroadcastAutoAttackStop() {
	if a.broadcastAutoAttackStop != nil {
		a.broadcastAutoAttackStop()
	}
}

// broadcast builds one frame lazily — only once a known observer capable of
// receiving frames is found — and hands every such receiver an independently
// owned copy, releasing the source frame afterwards. It is a no-op until
// SpawnBesideOwner has attached a world and SetFrameBuilder has installed the
// network-owned builder.
func (a *Actor) broadcast(build func() wire.Frame) {
	if a.world == nil || a.frames == nil {
		return
	}
	var frame wire.Frame
	built := false
	defer func() {
		if built {
			frame.Release()
		}
	}()
	a.world.ForEachKnown(a, func(o world.Tracked) {
		receiver, ok := o.(interface{ BroadcastFrame(wire.Frame) bool })
		if !ok {
			return
		}
		if !built {
			frame = build()
			built = true
		}
		owned, ok := wire.CopyFrame(frame)
		if ok {
			receiver.BroadcastFrame(owned)
		}
	})
}

// BroadcastFrame sends frame to every currently known observer capable of
// receiving one (i.e. a connected player session), from this summon's own
// known list. It takes ownership of frame and releases it. It is a no-op until
// SpawnBesideOwner has attached a world. Unlike the typed Broadcast* methods
// below, it carries an already-built frame and needs no FrameBuilder.
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
		owned, ok := wire.CopyFrame(frame)
		if ok {
			receiver.BroadcastFrame(owned)
		}
	})
}

func (a *Actor) BroadcastMove(event move.Event) error {
	a.broadcast(func() wire.Frame { return a.frames.Move(a.ObjectID(), event) })
	return nil
}

func (a *Actor) SyncPosition(position location.Location) {
	if a.world != nil {
		_ = a.world.Move(a, position.X, position.Y, position.Z)
	}
}

func (a *Actor) BroadcastStop() error {
	x, y, z := a.Position()
	a.broadcast(func() wire.Frame {
		return a.frames.Stop(a.ObjectID(), location.Location{X: x, Y: y, Z: z}, a.Heading())
	})
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
	a.broadcast(func() wire.Frame {
		return a.frames.MoveToPawn(a.ObjectID(), target.ObjectID(), distance, origin)
	})
	return nil
}

// BroadcastSelfSkillUse sends the cast-start animation of skillID at level
// with this summon as both caster and target, to every currently known
// observer capable of receiving one, matching the reference's
// summon.broadcastPacket(new MagicSkillUse(summon, summon, ...)) self-cast
// shape. It is a silent no-op until SpawnBesideOwner has attached a world and
// SetFrameBuilder has installed the network-owned builder.
func (a *Actor) BroadcastSelfSkillUse(skillID, level int32) error {
	x, y, z := a.Position()
	at := location.Location{X: x, Y: y, Z: z}
	a.broadcast(func() wire.Frame {
		return a.frames.SkillUse(a.ObjectID(), at, a.ObjectID(), at, skillID, level, 0, 0, false)
	})
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
