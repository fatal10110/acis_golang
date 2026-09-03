package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
// It reports whether inst was handled by this path, so the caller's
// equip-toggle fallback still answers the client for anything else.
func (l *GameClientLink) useItemAICast(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
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
		if live.SkillDisabled(actorcast.ReuseKey(def)) {
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(def.ID), int32(def.Level)))
			return true
		}
		selected := live.Target()
		if run != nil || itemAICastBusy(live) {
			if _, ok := actorcast.SelectTarget(live.Character, selected, def); !ok {
				sendMagicActionFailed(live)
				continue
			}
			live.deferItemAICast(inv, inst, def, selected)
			sendMagicActionFailed(live)
			continue
		}
		next, rejected, failed := l.beginItemAICast(live, inv, inst, tmpl, selected, def)
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
// in-flight swing or cast. Sit/stand transition states and scheduling a
// CAST after the current intention type are not modeled yet: #2191.
func itemAICastBusy(live *livePlayer) bool {
	if live.attack != nil && live.attack.AttackingNow() {
		return true
	}
	return live.cast != nil && live.cast.CastingNow()
}

// beginItemAICast starts and consumes one item-carried skill, sending its
// MagicSkillUse/gauge packets, but does not Schedule. rejected means the
// start gates refused the skill (the attached-skill loop continues). failed
// means the item could not be consumed after the cast had already opened
// (the loop stops).
func (l *GameClientLink) beginItemAICast(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template, selected world.Tracked, def modelskill.Definition) (run func(), rejected, failed bool) {
	beforeVitals := live.Vitals()
	controller := l.castController(live)
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Now:         time.Now(),
		Controller:  controller,
		Caster:      live.Character,
		Selected:    selected,
		Skill:       modelskill.Ref{ID: def.ID, Level: def.Level},
		Definitions: l.skills,
	})
	if err != nil {
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

	targetIDs := []int32{target.ObjectID()}
	return func() {
		controller.Schedule(plan, actorcast.Hooks{
			Launch: func() bool {
				if reason := actorcast.RevalidateLaunch(live.Character, target, def); reason != actorcast.LaunchAbortNone {
					sendLaunchAbort(live, reason)
					return false
				}
				l.broadcastLiveFrame(live, func() wire.Frame {
					return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), int32(def.ID), int32(def.Level), targetIDs)
				})
				return true
			},
			Hit: func() {
				result := actorcast.ApplyEffectsResult(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, target, def)
				l.sendSkillHandlerResult(live, result)
				l.syncCubicTargets(live, result, def)
				sendMagicStatusUpdate(live, beforeVitals)
			},
			Failed: func(err error) {
				sendMagicCastFailureReason(live, def, err)
				sendMagicStatusUpdate(live, beforeVitals)
			},
		})
	}, false, false
}

func (l *GameClientLink) finishDeferredItemAICast(live *livePlayer) bool {
	if live == nil || live.detached() {
		return false
	}
	itemCast := live.takeDeferredItemAICast()
	if itemCast == nil {
		return false
	}
	tmpl, _ := itemCast.inventory.Templates().Get(itemCast.item.TemplateID)
	run, rejected, failed := l.beginItemAICast(live, itemCast.inventory, itemCast.item, tmpl, itemCast.selected, itemCast.skill)
	if failed || rejected || run == nil {
		return false
	}
	run()
	return true
}
