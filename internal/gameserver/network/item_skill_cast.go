package network

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// useItemAICast runs each non-instant item-carried skill through the same
// Start/Launch/Hit/Finish sequence a player-initiated RequestMagicSkillUse
// drives, targeting the player's current selection. Resolving skills and
// consuming the item are itemhandler's decisions; this method sends the
// packets those decisions produce.
//
// Each skill first passes the item's own condition and reuse checks, which
// answer a refused skill and stop the rest. ctrl is the UseItem Ctrl
// modifier; it is the cast's force-use flag, and a queued skill keeps it.
//
// It reports whether inst was handled by this path, so the caller's
// equip-toggle fallback still answers the client for anything else.
func (l *GameClientLink) useItemAICast(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, ctrl bool) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return false
	}
	defs := itemhandler.ResolveAICastSkills(tmpl, l.skills)
	if len(defs) == 0 {
		return false
	}
	l.castItemSkills(live, inv, inst, defs, ctrl, true)
	return true
}

// castItemSkills casts defs one after another, as tryToCast would: the
// first eligible skill starts immediately; a later eligible skill is stored
// as the next CAST intention and runs when the in-flight action finishes. A
// later queue call overwrites an earlier one. carrier is the item carrying
// the casts, consumed as each one starts and before its start-of-cast
// costs; nil casts them as ordinary skills. itemGate runs an ItemSkills
// item's condition and reuse checks on each skill first.
//
// Schedule of a started skill is deferred until this function returns, so
// a later skill's reuse rejection cannot skip the first skill's timers,
// and a zero-delay launch cannot fire before later skills are queued.
//
// Each skill passes the player's pre-attempt gate, as a skill-bar request
// does: a skill that fails it is answered and neither queued nor started.
func (l *GameClientLink) castItemSkills(live *livePlayer, inv *itemcontainer.Inventory, carrier *item.Instance, defs []modelskill.Definition, ctrl, itemGate bool) {
	var run func()
	defer func() {
		if run != nil {
			run()
		}
	}()
	for _, def := range defs {
		if itemGate {
			if failed, ok := conditions.EvaluateSkill(def, live.Character, live.Target()); !ok {
				sendSkillConditionFailure(live, failed, def.ID)
				return
			}
			if live.SkillDisabled(actorcast.ReuseKey(def)) {
				live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(def.ID), int32(def.Level)))
				return
			}
		}
		selected := live.Target()
		if !l.attemptItemAICast(live, selected, def) {
			continue
		}
		// Past the pre-attempt gate the attached skill is the CAST
		// intention, started now or queued: it takes the attack's place.
		if live.combat != nil {
			live.combat.ReplaceWithCast()
		}
		live.endFollow()
		if run != nil || itemAICastBusy(live) {
			live.deferItemAICast(inv, carrier, def, selected, ctrl)
			sendMagicActionFailed(live)
			continue
		}
		next, rejected, failed := l.beginItemAICast(live, inv, carrier, selected, def, ctrl)
		if failed {
			return
		}
		if rejected {
			continue
		}
		run = next
	}
}

// itemAICastBusy reports whether a later attached skill must wait: an
// in-flight swing or cast, or a sit-down or stand-up transition.
//
// A CAST is otherwise queued only behind a current STAND intention, which
// lasts exactly as long as the stand-up transition, so no separate
// intention type is tracked for it.
func itemAICastBusy(live *livePlayer) bool {
	if live.attack != nil && live.attack.AttackingNow() {
		return true
	}
	if live.cast != nil && live.cast.CastingNow() {
		return true
	}
	return inPostureTransition(live)
}

// inPostureTransition reports whether live is still sitting down or standing
// up. A cast queued behind a swing or cast that ends inside the transition
// stays queued until PostureSettled runs it.
func inPostureTransition(live *livePlayer) bool {
	return live.SittingNow() || live.StandingNow()
}

// beginItemAICast starts one item-carried skill, sending its
// MagicSkillUse/gauge packets, but does not Schedule. The carrier, when
// there is one, is consumed and its shared reuse announced once the cast
// is claimed and before any start-of-cast cost; without one the cast is an
// ordinary skill cast and announces USE_S1. rejected means the start gates
// refused the skill (the attached-skill loop continues). failed means the
// carrier could not be consumed (the loop stops).
func (l *GameClientLink) beginItemAICast(live *livePlayer, inv *itemcontainer.Inventory, carrier *item.Instance, selected world.Tracked, def modelskill.Definition, ctrl bool) (run func(), rejected, failed bool) {
	controller := l.castController(live)
	var carrierLost bool
	var consumeCarrier func() error
	if carrier != nil {
		tmpl, _ := inv.Templates().Get(carrier.TemplateID)
		consumeCarrier = func() error {
			consumed := itemhandler.ConsumeAICastItem(itemhandler.ConsumeAICastItemRequest{
				Caster:     live.Character,
				Definition: def,
				Inventory:  inv,
				Item:       carrier,
				Template:   tmpl,
				Destroyer:  l.inventory,
			})
			if consumed.Err != nil {
				carrierLost = true
				return consumed.Err
			}
			if consumed.SharedReuseGroup >= 0 {
				live.SendFrame(serverpackets.FrameExUseSharedGroupItem(carrier.TemplateID, consumed.SharedReuseGroup, consumed.ReuseMillis, consumed.ReuseMillis))
			}
			return nil
		}
	}
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Controller:  controller,
		Caster:      live.Character,
		Selected:    selected,
		Skill:       modelskill.Ref{ID: def.ID, Level: def.Level},
		Definitions: l.skills,
		Ctrl:        ctrl,
		Hooks: actorcast.StartHooks{
			ResolveTarget:  l.resolveMagicSkillTarget,
			StopMovement:   l.stopMovementForCast(live),
			ConsumeCarrier: consumeCarrier,
		},
	})
	if err != nil {
		if carrierLost {
			sendItemConsumeFailure(live)
			return nil, false, true
		}
		if started.CanCastFailure && magicCastFailureReasonOnly(err) {
			sendMagicCastFailureReason(live, started.Definition, err)
			return nil, true, false
		}
		if errors.Is(err, actorcast.ErrInvalidTarget) && started.Rejection != skilltarget.CastRejectNone {
			sendTargetCastRejection(live, started.Rejection, started.Definition)
			return nil, true, false
		}
		if errors.Is(err, actorcast.ErrInvalidTarget) && started.Target == nil {
			// No final target: the request is dropped with ActionFailed
			// alone.
			sendMagicActionFailed(live)
			return nil, true, false
		}
		sendMagicCastFailure(live, started.Definition, err)
		return nil, true, false
	}
	target := started.Target
	plan := started.Plan

	if carrier == nil {
		l.broadcastCastStart(live, target, def, plan)
	} else {
		casterObject := skillCastObject(live)
		targetObject := skillCastObject(target)
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillUse(
				casterObject,
				targetObject,
				int32(def.ID),
				int32(def.Level),
				millis(plan.HitTime),
				millis(plan.ReuseDelay),
				false,
			)
		})
	}
	if plan.GaugeDuration > 0 {
		live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeBlue, millis(plan.GaugeDuration), millis(plan.GaugeDuration)))
	}

	handlers := l.castEffects()
	var affected []skilltarget.Actor
	return func() {
		controller.Schedule(plan, actorcast.Hooks{
			Launch: func() bool {
				var ok bool
				affected, ok = l.launchCastTargets(live, target, def)
				return ok
			},
			Hit: func() {
				l.applyCastHit(live, handlers, affected, def)
			},
			Failed: func(err error) {
				sendMagicCastFailureReason(live, def, err)
			},
		})
	}, false, false
}

// finishDeferredItemAICast runs the queued item cast, if any, and reports
// whether one was waiting. It passes the pre-attempt gate again first: a
// queued skill with no final target any more is dropped silently, and one
// the gate now refuses is answered with the reason alone. Either way the
// queued cast was the next intention, so it still reports true: the action
// that just ended does not resume or follow up. During a sit-down or
// stand-up the cast stays queued for PostureSettled.
func (l *GameClientLink) finishDeferredItemAICast(live *livePlayer) bool {
	if live == nil || live.detached() {
		return false
	}
	if inPostureTransition(live) {
		return live.hasDeferredItemAICast()
	}
	itemCast := live.takeDeferredItemAICast()
	if itemCast == nil {
		return false
	}
	l.resumeItemAICast(live, itemCast)
	return true
}

// resumeItemAICast starts a queued item cast taken at its action's end, if
// it still passes the gates.
func (l *GameClientLink) resumeItemAICast(live *livePlayer, itemCast *itemAICastIntention) {
	target := l.skillFinalTarget(live, itemCast.selected, itemCast.skill)
	if target == nil {
		return
	}
	if err := l.castController(live).CanPlayerAttemptItemCast(live.Character, target, itemCast.skill); err != nil {
		sendMagicCastFailureReason(live, itemCast.skill, err)
		return
	}
	run, rejected, failed := l.beginItemAICast(live, itemCast.inventory, itemCast.item, itemCast.selected, itemCast.skill, itemCast.ctrl)
	if failed || rejected || run == nil {
		return
	}
	run()
}

// attemptItemAICast runs one attached skill through the pre-attempt gate
// before it is queued or started. A dead caster and a skill with no final
// target get a bare ActionFailed; a gate failure gets its reason and
// ActionFailed. It reports whether the skill may go on.
func (l *GameClientLink) attemptItemAICast(live *livePlayer, selected world.Tracked, def modelskill.Definition) bool {
	if live.Character.Dead() {
		sendMagicActionFailed(live)
		return false
	}
	target := l.skillFinalTarget(live, selected, def)
	if target == nil {
		sendMagicActionFailed(live)
		return false
	}
	if err := l.castController(live).CanPlayerAttemptItemCast(live.Character, target, def); err != nil {
		sendMagicCastFailure(live, def, err)
		return false
	}
	return true
}

// skillFinalTarget is the creature def would be cast on given selected,
// before any cast condition is checked.
func (l *GameClientLink) skillFinalTarget(live *livePlayer, selected world.Tracked, def modelskill.Definition) skilltarget.Actor {
	if l.targets == nil {
		return nil
	}
	handler, ok := l.targets.Handler(def.Target)
	if !ok {
		return nil
	}
	selectedActor, _ := selected.(skilltarget.Actor)
	return handler.FinalTarget(live.Character, selectedActor, &def)
}
