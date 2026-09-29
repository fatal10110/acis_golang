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
// drives, targeting the player's current selection. The first eligible
// skill starts immediately; a later eligible skill is stored as the next
// CAST intention and runs when the in-flight action finishes. A later
// queue call overwrites an earlier one. Resolving skills and consuming the
// item are itemhandler's decisions; this method sends the packets those
// decisions produce. The item is consumed before the cast/launch packets
// go out, not only on a successful hit.
//
// Schedule of a started skill is deferred until this function returns, so
// a later skill's reuse rejection cannot skip the first skill's timers,
// and a zero-delay launch cannot fire before later skills are queued.
//
// Each skill first passes the player's pre-attempt gate, as a skill-bar
// request does: a skill that fails it is answered and neither queued nor
// started. ctrl is the UseItem Ctrl modifier; it is the cast's force-use
// flag, and a queued skill keeps it.
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

	var run func()
	defer func() {
		if run != nil {
			run()
		}
	}()
	for _, def := range defs {
		if failed, ok := conditions.EvaluateSkill(def, live.Character, live.Target()); !ok {
			sendSkillConditionFailure(live, failed, def.ID)
			return true
		}
		if live.SkillDisabled(actorcast.ReuseKey(def)) {
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(def.ID), int32(def.Level)))
			return true
		}
		selected := live.Target()
		if !l.attemptItemAICast(live, selected, def) {
			continue
		}
		if run != nil || itemAICastBusy(live) {
			live.deferItemAICast(inv, inst, def, selected, ctrl)
			sendMagicActionFailed(live)
			continue
		}
		next, rejected, failed := l.beginItemAICast(live, inv, inst, tmpl, selected, def, ctrl)
		if failed {
			return true
		}
		if rejected {
			continue
		}
		run = next
	}
	return true
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

// beginItemAICast starts and consumes one item-carried skill, sending its
// MagicSkillUse/gauge packets, but does not Schedule. rejected means the
// start gates refused the skill (the attached-skill loop continues). failed
// means the item could not be consumed after the cast had already opened
// (the loop stops).
func (l *GameClientLink) beginItemAICast(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template, selected world.Tracked, def modelskill.Definition, ctrl bool) (run func(), rejected, failed bool) {
	controller := l.castController(live)
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Controller:  controller,
		Caster:      live.Character,
		Selected:    selected,
		Skill:       modelskill.Ref{ID: def.ID, Level: def.Level},
		Definitions: l.skills,
		Ctrl:        ctrl,
		Hooks: actorcast.StartHooks{
			ResolveTarget: l.resolveMagicSkillTarget,
			StopMovement:  l.stopMovementForCast(live),
		},
	})
	if err != nil {
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

	consumed := itemhandler.ConsumeAICastItem(itemhandler.ConsumeAICastItemRequest{
		Controller: controller,
		Definition: def,
		Inventory:  inv,
		Item:       inst,
		Template:   tmpl,
		Destroyer:  l.inventory,
	})
	if consumed.Err != nil {
		sendItemConsumeFailure(live)
		return nil, false, true
	}
	if consumed.SharedReuseGroup >= 0 {
		live.SendFrame(serverpackets.FrameExUseSharedGroupItem(inst.TemplateID, consumed.SharedReuseGroup, consumed.ReuseMillis, consumed.ReuseMillis))
	}

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

// finishDeferredItemAICast runs the queued item cast, if any. It passes the
// pre-attempt gate again first: a queued skill with no final target any
// more is dropped silently, and one the gate now refuses is answered with
// the reason alone. During a sit-down or stand-up the cast stays queued for
// PostureSettled, and it reports true: it is still the next intention.
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
	target := l.skillFinalTarget(live, itemCast.selected, itemCast.skill)
	if target == nil {
		return false
	}
	if err := l.castController(live).CanPlayerAttemptItemCast(live.Character, target, itemCast.skill); err != nil {
		sendMagicCastFailureReason(live, itemCast.skill, err)
		return false
	}
	tmpl, _ := itemCast.inventory.Templates().Get(itemCast.item.TemplateID)
	run, rejected, failed := l.beginItemAICast(live, itemCast.inventory, itemCast.item, tmpl, itemCast.selected, itemCast.skill, itemCast.ctrl)
	if failed || rejected || run == nil {
		return false
	}
	run()
	return true
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
