package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// EffectPoint is a short-lived, invulnerable world actor a signet skill
// spawns to carry its periodic area effect: it has no HP pool, is never a
// legal attack or skill target, and exists only to host a ticking
// effect.List and broadcast its own skill-use packets to nearby observers.
type EffectPoint struct {
	world.Presence

	objectID int32
	Instance *Instance
	ownerID  int32

	effects *effect.List
	world   *world.State
	frames  FrameBuilder
	log     zerolog.Logger
}

// NewEffectPoint creates an unspawned EffectPoint from template, attributed
// to ownerID (the acting player's object id).
func NewEffectPoint(objectID int32, template *Template, ownerID int32) (*EffectPoint, error) {
	inst, err := NewInstance(objectID, template)
	if err != nil {
		return nil, err
	}
	ep := &EffectPoint{objectID: objectID, Instance: inst, ownerID: ownerID}
	ep.effects = effect.NewList(ep)
	return ep, nil
}

// ObjectID returns the actor's world object id.
func (ep *EffectPoint) ObjectID() int32 { return ep.objectID }

// OwnerID returns the acting player's object id this actor was spawned
// for.
func (ep *EffectPoint) OwnerID() int32 { return ep.ownerID }

// Dead always reports false: an EffectPoint carries no HP pool and spawns
// invulnerable.
func (ep *EffectPoint) Dead() bool { return false }

// CollisionRadius returns the actor's template collision radius, widening
// world.inRange scans by this actor's own body just like every other
// tracked NPC.
func (ep *EffectPoint) CollisionRadius() float64 { return ep.Instance.Template.CollisionRadius }

// EffectList returns the actor's own live effect list, driven by the
// server's shared effect scheduler exactly like any other actor.
func (ep *EffectPoint) EffectList() *effect.List { return ep.effects }

// AddStatFuncs, RemoveStatsByOwner, and MaxBuffCount satisfy
// effect.StatOwner; an EffectPoint carries no stats and no buff slots.
func (ep *EffectPoint) AddStatFuncs([]effect.Mod)          {}
func (ep *EffectPoint) RemoveStatsByOwner(effect.ModOwner) {}
func (ep *EffectPoint) MaxBuffCount() int                  { return 0 }

// SetWorld attaches the world state this actor spawns into and broadcasts
// through.
func (ep *EffectPoint) SetWorld(state *world.State) { ep.world = state }

// SetFrameBuilder records the network-layer hook that translates this
// actor's broadcast-worthy state changes into wire frames, keeping
// serverpackets and wire-encoding knowledge out of the model layer.
func (ep *EffectPoint) SetFrameBuilder(b FrameBuilder) { ep.frames = b }

// SetLogger records where a broadcast failure from this actor's own
// periodic tick (not routed through an AI think loop) is logged. The zero
// value discards it.
func (ep *EffectPoint) SetLogger(log zerolog.Logger) { ep.log = log }

// Spawn places the actor in the world at (x, y, z), facing heading. It is a
// no-op until SetWorld has been called.
func (ep *EffectPoint) Spawn(x, y, z, heading int) {
	if ep.world == nil {
		return
	}
	ep.world.Spawn(ep, x, y, z, heading)
}

// Despawn removes the actor from the world. It is a no-op until SetWorld
// has been called.
func (ep *EffectPoint) Despawn() {
	if ep.world == nil {
		return
	}
	ep.world.Despawn(ep)
}

// ForEachNearby calls fn for every world object within radius units of
// this actor, excluding itself. It is a no-op until SetWorld has been
// called.
func (ep *EffectPoint) ForEachNearby(radius int, fn func(world.Tracked)) {
	if ep.world == nil {
		return
	}
	ep.world.ForEachKnownInRadius(ep, radius, fn)
}

// skillCastTarget is the minimal surface a signet tick's found target must
// expose to appear as the target endpoint of a broadcast skill-use/launch
// packet pair.
type skillCastTarget interface {
	ObjectID() int32
	Position() (x, y, z int)
}

// BroadcastSkillUse sends a cast-start animation packet from this actor to
// target, to every currently known observer capable of receiving one. It
// is a no-op until SetWorld has been called.
func (ep *EffectPoint) BroadcastSkillUse(target skillCastTarget, skillID, level int32) error {
	if ep.world == nil {
		return ErrNoWorld
	}
	if ep.frames == nil {
		return ErrNoFrameBuilder
	}
	ax, ay, az := ep.Position()
	tx, ty, tz := target.Position()
	ep.broadcastFrame(ep.frames.SkillUse(
		ep.ObjectID(), location.Location{X: ax, Y: ay, Z: az},
		target.ObjectID(), location.Location{X: tx, Y: ty, Z: tz},
		skillID, level, 0, 0, false,
	))
	return nil
}

// BroadcastSkillLaunched sends the cast-launch target packet for skillID at
// level, listing targetIDs, to every currently known observer capable of
// receiving one. It is a no-op until SetWorld has been called.
func (ep *EffectPoint) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) error {
	if ep.world == nil {
		return ErrNoWorld
	}
	if ep.frames == nil {
		return ErrNoFrameBuilder
	}
	ep.broadcastFrame(ep.frames.SkillLaunched(ep.ObjectID(), skillID, level, targetIDs))
	return nil
}

func (ep *EffectPoint) broadcastFrame(fr wire.Frame) {
	ep.world.ForEachKnown(ep, func(o world.Tracked) {
		receiver, ok := o.(interface{ BroadcastFrame(wire.Frame) bool })
		if !ok {
			return
		}
		frame, ok := wire.CopyFrame(fr)
		if ok {
			receiver.BroadcastFrame(frame)
		}
	})
	fr.Release()
}
