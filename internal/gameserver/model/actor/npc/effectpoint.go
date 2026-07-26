package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// EffectPoint is a short-lived, invulnerable world actor a signet skill
// spawns to carry its periodic area effect: it has no HP pool, is never a
// legal attack or skill target, and exists only to host a ticking
// effect.List and broadcast its own skill-use packets to nearby observers.
// It mirrors the reference EffectPoint actor spawned by the L2SkillSignet/
// L2SkillSignetCasttime family.
type EffectPoint struct {
	world.Presence

	objectID int32
	Instance *Instance
	ownerID  int32

	effects *effect.List
	world   *world.State
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

// Dead always reports false: an EffectPoint carries no HP pool and the
// reference actor spawns invulnerable.
func (ep *EffectPoint) Dead() bool { return false }

// EffectList returns the actor's own live effect list, driven by the
// server's shared effect scheduler exactly like any other actor.
func (ep *EffectPoint) EffectList() *effect.List { return ep.effects }

// AddStatFuncs, RemoveStatsByOwner, and MaxBuffCount satisfy
// effect.StatOwner; an EffectPoint carries no stats and no buff slots.
func (ep *EffectPoint) AddStatFuncs([]basefunc.Func) {}
func (ep *EffectPoint) RemoveStatsByOwner(any)       {}
func (ep *EffectPoint) MaxBuffCount() int            { return 0 }

// SetWorld attaches the world state this actor spawns into and broadcasts
// through.
func (ep *EffectPoint) SetWorld(state *world.State) { ep.world = state }

// Spawn places the actor in the world at (x, y, z), facing heading. It is a
// no-op until SetWorld has been called.
func (ep *EffectPoint) Spawn(x, y, z, heading int) {
	if ep.world == nil {
		return
	}
	ep.world.Spawn(ep, x, y, z, heading)
}

// Despawn removes the actor from the world, mirroring the reference
// actor's deleteMe. It is a no-op until SetWorld has been called.
func (ep *EffectPoint) Despawn() {
	if ep.world == nil {
		return
	}
	ep.world.Despawn(ep)
}

// ForEachNearby calls fn for every world object within radius units of
// this actor, excluding itself, mirroring the reference actor's
// getKnownTypeInRadius. It is a no-op until SetWorld has been called.
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
func (ep *EffectPoint) BroadcastSkillUse(target skillCastTarget, skillID, level int32) {
	if ep.world == nil {
		return
	}
	ax, ay, az := ep.Position()
	tx, ty, tz := target.Position()
	ep.broadcastFrame(serverpackets.FrameMagicSkillUse(
		serverpackets.SkillCastObject{ObjectID: ep.ObjectID(), Location: location.Location{X: ax, Y: ay, Z: az}},
		serverpackets.SkillCastObject{ObjectID: target.ObjectID(), Location: location.Location{X: tx, Y: ty, Z: tz}},
		skillID, level, 0, 0, true,
	))
}

// BroadcastSkillLaunched sends the cast-launch target packet for skillID at
// level, listing targetIDs, to every currently known observer capable of
// receiving one. It is a no-op until SetWorld has been called.
func (ep *EffectPoint) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) {
	if ep.world == nil {
		return
	}
	ep.broadcastFrame(serverpackets.FrameMagicSkillLaunched(ep.ObjectID(), skillID, level, targetIDs))
}

func (ep *EffectPoint) broadcastFrame(fr wire.Frame) {
	ep.world.ForEachKnown(ep, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(fr)
	})
}
