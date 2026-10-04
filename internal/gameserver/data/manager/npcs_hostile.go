package manager

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

type locatedRef struct{ move.Actor }

type homePathRecoveryActor interface {
	GeoPathFailCount() int
	ResetGeoPathFailCount()
	AddGeoPathFailCount()
	TeleportTo(location.Location)
}

func (r *locatedRef) GeoPathFailCount() int {
	if a, ok := r.Actor.(homePathRecoveryActor); ok {
		return a.GeoPathFailCount()
	}
	return 0
}

func (r *locatedRef) ResetGeoPathFailCount() {
	if a, ok := r.Actor.(homePathRecoveryActor); ok {
		a.ResetGeoPathFailCount()
	}
}

func (r *locatedRef) AddGeoPathFailCount() {
	if a, ok := r.Actor.(homePathRecoveryActor); ok {
		a.AddGeoPathFailCount()
	}
}

func (r *locatedRef) TeleportTo(target location.Location) {
	if a, ok := r.Actor.(homePathRecoveryActor); ok {
		a.TeleportTo(target)
	}
}

func (r *locatedRef) OffensiveFollowLead() bool {
	actor, ok := r.Actor.(*npc.Hostile)
	return ok && actor.OffensiveFollowLead()
}

// IntentionMovesToTarget forwards the NPC's current intention's
// move-to-target flag; an unwired ref may always close in.
func (r *locatedRef) IntentionMovesToTarget() bool {
	actor, ok := r.Actor.(*npc.Hostile)
	return !ok || actor.IntentionMovesToTarget()
}

// CanSee forwards the NPC's line of sight; an unwired ref sees everything.
func (r *locatedRef) CanSee(target attackable.Combatant) bool {
	actor, ok := r.Actor.(*npc.Hostile)
	return !ok || actor.CanSee(target)
}

// Knows forwards the NPC's known list, which ends a chase walk whose target
// left it; an unwired ref knows everything.
func (r *locatedRef) Knows(target attackable.Combatant) bool {
	actor, ok := r.Actor.(*npc.Hostile)
	return !ok || actor.Knows(target)
}

type (
	creatureActorRef struct{ attack.CreatureActor }
	statOwnerRef     struct{ effect.StatOwner }
)

// walkerActorRef adapts a live Hostile plus its movement controller to
// task.WalkerActor. Hostile's own promoted Position() returns (x, y, z int)
// for its other controllers, not WalkerActor's location.Location shape, so
// this narrows/adapts the mismatched methods the same way locatedRef and
// creatureActorRef adapt Hostile for its move/attack controllers; every
// other WalkerActor method (ObjectID, GeoPathFailCount, SayNPCString, ...)
// promotes straight through from the embedded Hostile.
type walkerActorRef struct {
	*npc.Hostile
	moveCtl *move.Controller

	// routeMove records whether the move currently in flight (or the one
	// that just completed) was issued by the walker task itself, so the
	// shared arrived hook (see newLiveHostile) can tell an actual route
	// arrival from any other arrival on the same Hostile — offensive-follow
	// chase and MoveHome leash-return use the same underlying
	// move.Controller and fire the identical hook. Set true here, cleared
	// by routeAwareMoveController whenever AI starts a non-route move.
	// Arrival only continues route-node logic when the current AI intention
	// is MOVE_ROUTE.
	routeMove *atomic.Bool
}

func (r *walkerActorRef) Position() location.Location {
	x, y, z := r.Hostile.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func (r *walkerActorRef) Moving() bool {
	return r.Hostile.Move().Moving()
}

func (r *walkerActorRef) MoveToLocation(target location.Location) (event.Move, error) {
	r.routeMove.Store(true)
	return r.moveCtl.MoveToLocationEvent(target)
}

func (r *walkerActorRef) TeleportTo(target location.Location) {
	r.Hostile.TeleportTo(target)
}

// routeAwareMoveController wraps a Hostile's ai.MoveController so that any
// AI-initiated movement other than route walking (offensive-follow chase,
// leash return-home, random walk and escort moves) clears routeMove before it starts — otherwise the
// arrived hook would treat that move's completion as a route arrival too,
// since it fires through the same move.Controller and task.Walker.Arrived
// has no intention of its own to check.
type routeAwareMoveController struct {
	ai.MoveController
	routeMove *atomic.Bool
}

func (r routeAwareMoveController) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	r.routeMove.Store(false)
	return r.MoveController.MaybeStartOffensiveFollow(target, attackRange)
}

func (r routeAwareMoveController) MoveHome(home location.Location) error {
	r.routeMove.Store(false)
	return r.MoveController.MoveHome(home)
}

func (r routeAwareMoveController) MoveToLocation(dest location.Location) (bool, error) {
	r.routeMove.Store(false)
	return r.MoveController.MoveToLocation(dest)
}

func (r routeAwareMoveController) CanMoveTo(target location.Location) bool {
	g, ok := r.MoveController.(*move.Controller)
	return !ok || g.CanMoveTo(target)
}

// newLiveHostile builds a live Hostile for inst, wiring a real movement
// controller (over the Hostile's lifetime movement state) and a real attack
// controller, resolving their mutual construction-order dependency on the
// finished Hostile via locatedRef/creatureActorRef/statOwnerRef.
func newLiveHostile(inst *npc.Instance, speed float64, geo move.Geo, positions *task.PositionUpdates, log zerolog.Logger, castDefs actorcast.Definitions, castEffects actorcast.EffectHandlers, walker *task.Walker, maxBuffsAmount, maxGeoPathFailCount int, zones *zone.Index, effects effect.Env, queue *sim.Queue) (*npc.Hostile, *walkerActorRef, error) {
	control := &hostileControl{walker: walker, log: log, chance: castEffects.Chance}
	statRef := &statOwnerRef{}
	live, err := creature.NewLive(inst.Home, speed, geo, statRef, effect.WithEnv(effects))
	if err != nil {
		return nil, nil, err
	}
	live.SetQueue(queue)
	if zones != nil {
		live.Move().SetWaterSurface(waterSurface(zones))
		// The NPC swims while its water zones hold it, not wherever the
		// water query finds it.
		live.Move().UseCreatureZoneSwim()
	}

	locRef := &locatedRef{}
	moveCtl, err := move.NewController(live.Move(), locRef, control)
	if err != nil {
		return nil, nil, err
	}
	moveCtl.SetPositionUpdates(positions)

	actorRef := &creatureActorRef{}
	attackCtl := attack.NewAttackable(actorRef, control)
	attackCtl.SetQueue(queue)

	routeMove := &atomic.Bool{}
	hostile, err := npc.NewHostile(inst, live, routeAwareMoveController{MoveController: moveCtl, routeMove: routeMove}, attackCtl, castDefs)
	if err != nil {
		return nil, nil, err
	}
	hostile.SetMaxBuffsAmount(maxBuffsAmount)
	hostile.SetMaxGeoPathFailCount(maxGeoPathFailCount)
	if zones != nil {
		hostile.SetWaterZone(func(at location.Location) bool {
			_, ok := zone.FindAt[*zone.Water](zones, at.X, at.Y, at.Z)
			return ok
		})
	}
	hostile.SetZones(zones)

	locRef.Actor = hostile
	actorRef.CreatureActor = hostile
	statRef.StatOwner = hostile

	// Wire the AI-cast seam (issue #1612): a nil castDefs (an existing
	// harness that hasn't loaded skill data) leaves the AI loop with no
	// CastController, matching ai.Attackable's existing "no skills to
	// cast" no-op contract for IntentionCast — the same nil-safe pattern
	// SummonActor's caller relies on before l.skills is ready.
	if castDefs != nil {
		castController := actorcast.NewController(actorcast.HostileActor{Hostile: hostile}, control)
		castController.SetQueue(queue)
		aiController := &actorcast.AIController{
			Controller:  castController,
			Definitions: castDefs,
			Effects:     castEffects,
			Caster:      hostile,
			// A packet sent to a non-Player, non-Summon-owner caster goes
			// nowhere, so caster-addressed messages (ATTACK_FAILED,
			// MISSED_TARGET, ...) still have no forward target here. But
			// target-addressed messages (MagicResist, ManaDrain) are
			// delivered by ID lookup against the real target independent of
			// caster type,
			// so OnHitResult is wired to the boot-provided delivery hook
			// (issue #2350) rather than left unset.
			OnHitResult: castEffects.OnHitResult,
		}
		hostile.AI().SetCastController(aiController)
		hostile.SetCastController(castController)
	}

	walkerRef := &walkerActorRef{Hostile: hostile, moveCtl: moveCtl, routeMove: routeMove}

	control.hostile, control.move, control.walkerRef, control.routeMove = hostile, moveCtl, walkerRef, routeMove
	return hostile, walkerRef, nil
}

// hostileControl reacts to a live hostile NPC's controller events: it
// re-evaluates the AI loop as soon as a swing or its hit animation
// finishes, or a cast ends, rather than waiting for the next fixed AI tick,
// and closes an aborted AI cast with its cancel animation. An arrival,
// blocked or not, only settles the arrival state: the next intention step,
// such as the swing at the end of a chase leg, waits for the next RunAI or
// task.AITick.
// newLiveHostile fills it before the NPC is published.
type hostileControl struct {
	hostile   *npc.Hostile
	move      *move.Controller
	walker    *task.Walker
	walkerRef *walkerActorRef
	routeMove *atomic.Bool
	log       zerolog.Logger
	// chance runs the chance procs of the NPC's landed hits.
	chance *actorcast.ChanceProcs
}

// Emit maps one controller event to the NPC's AI and broadcasts.
func (c *hostileControl) Emit(ev event.Event) {
	switch e := ev.(type) {
	case event.HitLanded:
		c.chance.AttackHit(c.hostile, e)
	case event.AttackStanceRequested:
		c.hostile.EnterAttackStance()
	case event.Arrived:
		// CreatureMove tracks position for its own timing only; push the
		// arrived position into the world-grid presence range checks
		// actually read, or the next AI pass runs against a stale position.
		c.hostile.SyncPosition(c.move.Position())
		// A move's end revalidates the zones at once.
		c.hostile.SettleZones()
		// Only an arrival the walker task itself just moved toward counts as
		// a route arrival — offensive-follow chase and MoveHome arrive the
		// same way and must not advance/reissue the patrol route.
		if c.walker != nil && c.routeMove.Load() {
			if err := c.walker.Arrived(c.walkerRef); err != nil {
				c.log.Warn().Err(err).Msg("task: walker arrived")
			}
		}
		c.hostile.AI().Arrived()
	case event.MoveBlocked:
		c.move.BroadcastBlockedCorrection()
		c.hostile.AI().ArrivedBlocked()
	case event.AttackFinished:
		// A swing finishing re-runs desire selection, then continues an
		// out-of-control NPC's current intention; a bow's reuse ending only
		// continues the current intention.
		if e.BowReuse {
			c.think()
			return
		}
		if err := c.hostile.AttackFinished(); err != nil {
			c.log.Warn().Err(err).Msg("ai: hostile attack finished")
		}
	case event.AttackRethink:
		c.runAI()
	case event.CastFinished:
		if err := c.hostile.CastFinished(e.Interrupted); err != nil {
			c.log.Warn().Err(err).Msg("ai: hostile cast finished")
		}
	case event.CastAborted:
		// Every AI cast abort path (Launch revalidation failure,
		// insufficient MP/HP at Hit, a damage-break interrupt) routes
		// through Controller.Stop/Interrupt, which reports an abort only when
		// a cast was actually in flight — MagicSkillCanceled is broadcast
		// only behind that same casting-now guard, for an NPC as for any
		// other creature.
		c.hostile.BroadcastSkillCanceled(c.hostile.ObjectID())
	}
}

func (c *hostileControl) think() {
	if err := c.hostile.Think(); err != nil {
		c.log.Warn().Err(err).Msg("ai: hostile think")
	}
}

func (c *hostileControl) runAI() {
	if err := c.hostile.RunAI(); err != nil {
		c.log.Warn().Err(err).Msg("ai: hostile run")
	}
}

// walkerWalkModeIDs are the walker template ids that spawn in walk stance
// instead of every other NPC's default run stance.
var walkerWalkModeIDs = map[int32]bool{
	31357: true, 31358: true, 31359: true, 31360: true, 31362: true,
	31364: true, 31365: true, 31525: true, 32072: true, 32128: true,
}

// startWalkerRoute registers ref for route walking if inst's template alias
// resolves in walkerRoutes.xml (every spawned NPC whose template alias has
// route data gets an immediate route-move desire; both the route name and
// its per-NPC key are that alias). The caller must have
// already placed ref's Hostile into world.State — Walker only ticks actors
// it can find in-region, so calling this before the spawn lands is a
// silent no-op forever, not a delayed start. Most templates have no alias,
// or an alias with no route data — those are skipped silently via HasRoute.
// Past that check StartRoute fails only on the first move; the route stays
// registered and Walker's tick retries it, logging at Error if it keeps
// failing.
func startWalkerRoute(walker *task.Walker, ref *walkerActorRef, inst *npc.Instance, log zerolog.Logger) {
	alias := inst.Template.Alias
	if walker == nil || alias == "" || !walker.HasRoute(alias, alias) {
		return
	}
	if err := walker.StartRoute(ref, alias, alias); err != nil {
		log.Debug().Err(err).Str("alias", alias).Msg("npc: walker route first move failed, retrying on tick")
	}
}

// deathRewards applies one victim's live death rewards at its position at
// the moment of death, rather than a position fixed when it spawned —
// hostile NPCs can move (offensive follow) between spawning and dying.
