package manager

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

type locatedRef struct{ move.Actor }
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
}

func (r *walkerActorRef) Position() location.Location {
	x, y, z := r.Hostile.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func (r *walkerActorRef) Moving() bool {
	return r.Hostile.Move().Moving()
}

func (r *walkerActorRef) MoveToLocation(target location.Location) (move.Event, error) {
	return r.moveCtl.MoveToLocationEvent(target)
}

func (r *walkerActorRef) TeleportTo(target location.Location) {
	r.Hostile.SetXYZ(target.X, target.Y, target.Z)
	r.Hostile.BroadcastPosition()
}

// newLiveHostile builds a live Hostile for inst, wiring a real movement
// controller (over the Hostile's lifetime movement state) and a real attack
// controller, resolving their mutual construction-order dependency on the
// finished Hostile via locatedRef/creatureActorRef/statOwnerRef.
func newLiveHostile(inst *npc.Instance, speed float64, geo move.Geo, positions *task.PositionUpdates, log zerolog.Logger, castDefs actorcast.Definitions, castEffects actorcast.EffectHandlers, walker *task.Walker) (*npc.Hostile, *walkerActorRef, error) {
	statRef := &statOwnerRef{}
	live, err := creature.NewLive(inst.Home, speed, geo, statRef)
	if err != nil {
		return nil, nil, err
	}

	locRef := &locatedRef{}
	moveCtl, err := move.NewController(live.Move(), locRef)
	if err != nil {
		return nil, nil, err
	}
	moveCtl.SetPositionUpdates(positions)

	actorRef := &creatureActorRef{}
	attackCtl := attack.NewAttackable(actorRef)
	live.Move().SetLogger(log)
	attackCtl.SetLogger(log)

	hostile, err := npc.NewHostile(inst, live, moveCtl, attackCtl)
	if err != nil {
		return nil, nil, err
	}
	hostile.SetLogger(log)
	if los, ok := geo.(npc.LineOfSight); ok {
		hostile.SetLineOfSight(los)
	}

	locRef.Actor = hostile
	actorRef.CreatureActor = hostile
	statRef.StatOwner = hostile

	// Wire the AI-cast seam (issue #1612): a nil castDefs (an existing
	// harness that hasn't loaded skill data) leaves the AI loop with no
	// CastController, matching ai.Attackable's existing "no skills to
	// cast" no-op contract for IntentionCast — the same nil-safe pattern
	// SummonActor's caller relies on before l.skills is ready.
	if castDefs != nil {
		castController := actorcast.NewController(actorcast.HostileActor{Hostile: hostile})
		castController.SetLogger(log)
		aiController := &actorcast.AIController{
			Controller:  castController,
			Definitions: castDefs,
			Effects:     castEffects,
			Caster:      hostile,
			// OnHitResult is left unset: Creature.sendPacket is a no-op in
			// the reference for a non-Player, non-Summon-owner caster, so a
			// hostile-NPC cast's Hit-phase result has no forward target
			// (matching AIController's own OnHitResult doc, and
			// summon_spawn.go's Summon-only OnHitResult wiring).
		}
		hostile.AI().SetCastController(aiController)
	}

	walkerRef := &walkerActorRef{Hostile: hostile, moveCtl: moveCtl}

	// Re-evaluate the AI loop as soon as a chase leg completes or a swing
	// finishes, rather than waiting for the next fixed AI tick — otherwise
	// a hostile NPC only closes distance on, or re-attacks, its target once
	// per task.AITick. CreatureMove tracks position for its own timing only;
	// the arrived hook must push that position into the world-grid presence
	// range checks actually read before re-thinking, or the AI loop re-runs
	// against a stale position forever.
	moveCtl.SetArrived(func() {
		pos := moveCtl.Position()
		hostile.SyncPosition(pos)
		// aCis NpcAI.onEvtArrived: an arrival that lands exactly back on the
		// spawn point restores the spawn heading, regardless of what the NPC
		// last faced while moving.
		if inst.HasHome && pos == inst.Home {
			hostile.SetHeading(inst.SpawnHeading)
		}
		if walker != nil {
			if err := walker.Arrived(walkerRef); err != nil {
				log.Warn().Err(err).Msg("task: walker arrived")
			}
		}
		if err := hostile.Think(); err != nil {
			log.Warn().Err(err).Msg("ai: hostile think")
		}
	})
	attackCtl.SetFinished(func() {
		if err := hostile.Think(); err != nil {
			log.Warn().Err(err).Msg("ai: hostile think")
		}
	})

	return hostile, walkerRef, nil
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
