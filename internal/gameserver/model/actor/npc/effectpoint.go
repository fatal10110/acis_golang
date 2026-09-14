package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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
	sink    event.Sink
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

// Attach installs rt's World (the world this actor spawns into), Log (where
// a failure from its own periodic tick is logged) and Sink. Call it once,
// before Spawn; the other Runtime fields do not apply to an effect point.
func (ep *EffectPoint) Attach(rt Runtime) {
	ep.world = rt.World
	ep.log = rt.Log
	ep.sink = rt.Sink
}

// Spawn places the actor in the world at (x, y, z), facing heading. It is a
// no-op until Attach has installed a world.
func (ep *EffectPoint) Spawn(x, y, z, heading int) {
	if ep.world == nil {
		return
	}
	ep.world.Spawn(ep, x, y, z, heading)
}

// Despawn removes the actor from the world. It is a no-op until Attach has
// installed a world.
func (ep *EffectPoint) Despawn() {
	if ep.world == nil {
		return
	}
	ep.world.Despawn(ep)
	// Stop the periodic effect sweep from reaching this signet point's
	// list: it exists only to host that list, so leaving the list
	// registered after Despawn would tick a signet forever past its
	// caster's control.
	ep.effects.Untrack()
}

// ForEachNearby calls fn for every world object within radius units of
// this actor, excluding itself. It is a no-op until Attach installs a
// world.
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

// BroadcastSkillUse reports a cast-start animation from this actor to target.
// It is a no-op until Attach has installed a world.
func (ep *EffectPoint) BroadcastSkillUse(target skillCastTarget, skillID, level int32) error {
	if ep.world == nil {
		return ErrNoWorld
	}
	if ep.sink == nil {
		return nil
	}
	ax, ay, az := ep.Position()
	tx, ty, tz := target.Position()
	ep.sink.Emit(event.MagicSkillUse{
		CasterID: ep.ObjectID(), CasterAt: location.Location{X: ax, Y: ay, Z: az},
		TargetID: target.ObjectID(), TargetAt: location.Location{X: tx, Y: ty, Z: tz},
		SkillID: skillID, Level: level,
	})
	return nil
}

// BroadcastSkillLaunched reports the cast launch of skillID at level onto
// targetIDs. It is a no-op until Attach has installed a world.
func (ep *EffectPoint) BroadcastSkillLaunched(skillID, level int32, targetIDs []int32) error {
	if ep.world == nil {
		return ErrNoWorld
	}
	if ep.sink != nil {
		ep.sink.Emit(event.SkillLaunched{SkillID: skillID, Level: level, TargetIDs: targetIDs})
	}
	return nil
}
