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

type creatureActorRef struct{ attack.CreatureActor }
type statOwnerRef struct{ effect.StatOwner }

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
	// Mirrors aCis NpcAI.onEvtArrived's own gate: it only continues
	// route-node logic when the current AI intention is MOVE_ROUTE.
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
	control := &hostileControl{walker: walker, log: log}
	statRef := &statOwnerRef{}
	live, err := creature.NewLive(inst.Home, speed, geo, statRef, effect.WithEnv(effects))
	if err != nil {
		return nil, nil, err
	}
	if queue != nil {
		live.SetQueue(queue)
	}
	if zones != nil {
		live.Move().SetWaterSurface(func(position location.Location, groundZ int) (int, bool) {
			water, ok := zone.FindAt[*zone.Water](zones, position.X, position.Y, position.Z)
			if !ok || groundZ-water.WaterLevel() >= -20 {
				return 0, false
			}
			return water.WaterLevel(), true
		})
	}

	locRef := &locatedRef{}
	moveCtl, err := move.NewController(live.Move(), locRef, control)
	if err != nil {
		return nil, nil, err
	}
	moveCtl.SetPositionUpdates(positions)

	actorRef := &creatureActorRef{}
	attackCtl := attack.NewAttackable(actorRef, control)
	if queue != nil {
		attackCtl.SetQueue(queue)
	}

	routeMove := &atomic.Bool{}
	hostile, err := npc.NewHostile(inst, live, routeAwareMoveController{MoveController: moveCtl, routeMove: routeMove}, attackCtl, castDefs)
	if err != nil {
		return nil, nil, err
	}
	hostile.SetMaxBuffsAmount(maxBuffsAmount)
	hostile.SetMaxGeoPathFailCount(maxGeoPathFailCount)

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
		if queue != nil {
			castController.SetQueue(queue)
		}
		aiController := &actorcast.AIController{
			Controller:  castController,
			Definitions: castDefs,
			Effects:     castEffects,
			Caster:      hostile,
			// Creature.sendPacket is a no-op in the reference for a
			// non-Player, non-Summon-owner caster, so caster-addressed
			// messages (ATTACK_FAILED, MISSED_TARGET, ...) still have no
			// forward target here. But target-addressed messages
			// (MagicResist, ManaDrain) are delivered by ID lookup against
			// the real target independent of caster type (Manadam.java:68),
			// so OnHitResult is wired to the boot-provided delivery hook
			// (issue #2350) rather than left unset.
			OnHitResult: castEffects.OnHitResult,
		}
		hostile.AI().SetCastController(aiController)
	}

	walkerRef := &walkerActorRef{Hostile: hostile, moveCtl: moveCtl, routeMove: routeMove}

	control.hostile, control.move, control.walkerRef, control.routeMove = hostile, moveCtl, walkerRef, routeMove
	return hostile, walkerRef, nil
}

// hostileControl reacts to a live hostile NPC's controller events: it
// re-evaluates the AI loop as soon as a chase leg completes or a swing
// finishes, rather than waiting for the next fixed AI tick — otherwise a
// hostile NPC only closes distance on, or re-attacks, its target once per
// task.AITick — and closes an aborted AI cast with its cancel animation.
// newLiveHostile fills it before the NPC is published.
type hostileControl struct {
	hostile   *npc.Hostile
	move      *move.Controller
	walker    *task.Walker
	walkerRef *walkerActorRef
	routeMove *atomic.Bool
	log       zerolog.Logger
}

// Emit maps one controller event to the NPC's AI and broadcasts.
func (c *hostileControl) Emit(ev event.Event) {
	switch ev.(type) {
	case event.Arrived:
		// CreatureMove tracks position for its own timing only; push the
		// arrived position into the world-grid presence range checks
		// actually read before re-thinking, or the AI loop re-runs against a
		// stale position forever.
		c.hostile.SyncPosition(c.move.Position())
		// Only an arrival the walker task itself just moved toward counts as
		// a route arrival — offensive-follow chase and MoveHome arrive the
		// same way and must not advance/reissue the patrol route.
		if c.walker != nil && c.routeMove.Load() {
			if err := c.walker.Arrived(c.walkerRef); err != nil {
				c.log.Warn().Err(err).Msg("task: walker arrived")
			}
		}
		c.hostile.AI().Arrived()
		c.think()
	case event.MoveBlocked:
		c.move.BroadcastBlockedCorrection()
		c.hostile.AI().ArrivedBlocked()
		c.think()
	case event.AttackFinished:
		c.think()
	case event.CastAborted:
		// Every AI cast abort path (Launch revalidation failure,
		// insufficient MP/HP at Hit, a damage-break interrupt) routes
		// through Controller.Stop/Interrupt, which reports an abort only when
		// a cast was actually in flight — matching CreatureCast.stop()
		// broadcasting MagicSkillCanceled behind the same isCastingNow()
		// guard (CreatureCast.java:416-419), inherited unmodified by NpcCast.
		c.hostile.BroadcastSkillCanceled(c.hostile.ObjectID())
	}
}

func (c *hostileControl) think() {
	if err := c.hostile.Think(); err != nil {
		c.log.Warn().Err(err).Msg("ai: hostile think")
	}
}

// walkerWalkModeIDs are the template ids aCis Walkers.java's onCreated forces
// into walk stance (setWalkOrRun(false)) instead of every other NPC's
// default run stance; matches Walkers.java's WALKING_NPCS constant.
var walkerWalkModeIDs = map[int32]bool{
	31357: true, 31358: true, 31359: true, 31360: true, 31362: true,
	31364: true, 31365: true, 31525: true, 32072: true, 32128: true,
}

// startWalkerRoute registers ref for route walking if inst's template alias
// resolves in walkerRoutes.xml (aCis Walkers.java: every spawned NPC whose
// template alias has route data gets an immediate route-move desire; both
// the route name and its per-NPC key are that alias). The caller must have
// already placed ref's Hostile into world.State — Walker only ticks actors
// it can find in-region, so calling this before the spawn lands is a
// silent no-op forever, not a delayed start. Most templates have no alias,
// or an alias with no route data — StartRoute's "route not found" error is
// the expected, silent outcome for those, not a fault.
func startWalkerRoute(walker *task.Walker, ref *walkerActorRef, inst *npc.Instance, log zerolog.Logger) {
	if walker == nil || inst.Template.Alias == "" {
		return
	}
	if err := walker.StartRoute(ref, inst.Template.Alias, inst.Template.Alias); err != nil {
		log.Debug().Err(err).Str("alias", inst.Template.Alias).Msg("npc: not a route walker")
	}
}

// deathRewards applies one victim's live death rewards at its position at
// the moment of death, rather than a position fixed when it spawned —
// hostile NPCs can move (offensive follow) between spawning and dying.
