package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// nothingDesireDecay is how much weight every do-nothing desire loses each
// third AI tick.
const nothingDesireDecay = 0.5

// AddMoveToDesire asks f to walk to loc with weight; an equal desire (one
// within 20 units on the ground and 30 in height) already queued gains the
// weight instead. It is refused, reporting false, when f cannot move or no
// straight geodata walk from where f stands reaches loc. Safe from any
// goroutine.
func (f *Folk) AddMoveToDesire(loc location.Location, weight float64) bool {
	m := f.motion
	if m == nil || f.MovementDisabled() || !m.ctl.CanMoveTo(loc) {
		return false
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionMoveTo, Location: loc, Weight: weight, QueuedAt: f.now()})
	return true
}

// AddWanderDesire asks f to wander with weight; an equal desire already
// queued gains the weight instead. A civilian NPC has no wander step: while
// the desire outweighs the rest, f stands, and desire selection does not
// take up a second wander over it. timer is carried with the desire but
// times nothing. Safe from any goroutine.
func (f *Folk) AddWanderDesire(timer int, weight float64) {
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionWander, Timer: timer, Weight: weight, QueuedAt: f.now()})
}

// AddDoNothingDesire asks f to keep still with weight; an equal desire
// already queued gains the weight instead. Its weight decays as f's AI
// runs; timer is carried with the desire but times nothing. Safe from any
// goroutine.
func (f *Folk) AddDoNothingDesire(timer int, weight float64) {
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionNothing, Timer: timer, Weight: weight, QueuedAt: f.now()})
}

// AddSocialDesire asks f to play social animation id with weight; an equal
// desire (same id) already queued gains the weight instead. Once played,
// desire selection holds for timer milliseconds. Refused while f's AI
// sleeps. Safe from any goroutine.
func (f *Folk) AddSocialDesire(id, timer int, weight float64) {
	if f.AISleeping() {
		return
	}
	f.cast.desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionSocial, ItemObjectID: int32(id), Timer: timer, Weight: weight, QueuedAt: f.now()})
}

// AISleeping reports whether the AI task leaves f alone: it is dead, out
// of the world, or in a region no player keeps active.
func (f *Folk) AISleeping() bool {
	if f.Dead() {
		return true
	}
	if f.world == nil {
		return false
	}
	placed, active := f.world.RegionActivity(f)
	return !placed || !active
}

// CurrentIntention returns the kind of the desire f's AI last took up, and
// IntentionIdle once it idles or a cast it took up ends. Safe from any
// goroutine.
func (f *Folk) CurrentIntention() ai.Intention {
	if cur := f.currentDesire(); cur != nil {
		return cur.Kind
	}
	return ai.IntentionIdle
}

// socialHeld reports whether the hold a social animation put on desire
// selection is still running.
func (f *Folk) socialHeld() bool {
	return f.now().UnixMilli() < f.lastSocial.Load()
}

// act takes the step of d, the desire f's AI just took up. Wander and
// do-nothing have no step on a civilian NPC.
func (f *Folk) act(d *ai.Desire) {
	switch d.Kind {
	case ai.IntentionCast:
		f.thinkCast(d)
	case ai.IntentionMoveTo:
		f.thinkMoveTo(d)
	case ai.IntentionSocial:
		f.thinkSocial(d)
	}
}

// thinkMoveTo is the walk step: f already standing on the point opens the
// move-finished point and drops the walk's desire; otherwise f, when it can
// act and move, walks there.
func (f *Folk) thinkMoveTo(d *ai.Desire) {
	x, y, z := f.Position()
	if (location.Location{X: x, Y: y, Z: z}) == d.Location {
		f.AtHookPoint(ai.HookMoveFinished)
		f.cast.desires.RemoveIf(d.Equal)
		return
	}
	if f.denyAIAction() || f.motion == nil || f.MovementDisabled() {
		return
	}
	_, _ = f.motion.ctl.MoveToLocation(d.Location)
}

// thinkSocial is the social step: the desire leaves the queue, and an f
// that can act holds desire selection for the social's timer, stops and
// plays the animation. The hold shares its stamp with the talk animation,
// which waits it out.
func (f *Folk) thinkSocial(d *ai.Desire) {
	f.cast.desires.RemoveIf(d.Equal)
	if f.denyAIAction() {
		return
	}
	f.lastSocial.Store(f.now().Add(time.Duration(d.Timer) * time.Millisecond).UnixMilli())
	f.stopMoving()
	f.emit(event.SocialAction{ID: d.ItemObjectID})
}

// arrived is f's AI on a walk reaching its destination, on f's queue: a
// walk to a point opens the move-finished point, then drops its desire; a
// wander drops its desire.
func (f *Folk) arrived() {
	cur := f.currentDesire()
	if cur == nil {
		return
	}
	switch cur.Kind {
	case ai.IntentionMoveTo:
		f.AtHookPoint(ai.HookMoveFinished)
		f.cast.desires.RemoveIf(cur.Equal)
	case ai.IntentionWander:
		f.cast.desires.RemoveIf(cur.Equal)
	}
}

// arrivedBlocked is f's AI on a walk stopped short by a blocked path: a
// walk to a point or a wander drops its desire, with no move-finished
// point.
func (f *Folk) arrivedBlocked() {
	if cur := f.currentDesire(); cur != nil && (cur.Kind == ai.IntentionMoveTo || cur.Kind == ai.IntentionWander) {
		f.cast.desires.RemoveIf(cur.Equal)
	}
}

// inCastReach reports whether target stands strictly within castRange plus
// both collision radii of f, measured flat; the reach is truncated to
// whole units first.
func (f *Folk) inCastReach(target attackable.Combatant, castRange int) bool {
	reach := int(float64(castRange) + f.CollisionRadius() + target.CollisionRadius())
	ox, oy, oz := f.Position()
	tx, ty, tz := target.Position()
	from := location.Location{X: ox, Y: oy, Z: oz}
	return from.Distance2D(location.Location{X: tx, Y: ty, Z: tz}) < float64(reach)
}
