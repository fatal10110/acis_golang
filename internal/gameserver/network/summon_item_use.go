package network

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	skillref "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// SummonItemsHandler is the etc-item handler name carried by a pet collar.
const SummonItemsHandler = "SummonItems"

const (
	summonItemTypeDecorative = 0
	summonItemTypePet        = 1
	summonItemTypeWyvern     = 2
)

const (
	decorativeSummonRadius         = 1200
	systemMessageCannotSummonAgain = 1142
)

// summonCreatureSkillRef is the fixed SUMMON_CREATURE skill id/level every
// pet-collar item casts, independent of which collar was used — the collar
// only selects the npc template via SummonItemData.
var summonCreatureSkillRef = skillref.Ref{ID: 2046, Level: 1}

// useSummonItem triggers the SUMMON_CREATURE cast for a pet-collar item,
// driving it through the same timed
// Launch/Hit/Finish cast sequence useItemAICast uses for any other
// item-carried skill. Unlike a consumable, the collar itself is never
// destroyed — it stays the pet's persistent identity: the summon looks the
// pet up by control-item object id.
//
// It reports whether inst was handled by this path, so the caller's
// equip-toggle fallback still answers the client for anything else.
func (l *GameClientLink) useSummonItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil || l.summonItems == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != SummonItemsHandler {
		return false
	}
	summonItem, ok := l.summonItems.Item(inst.TemplateID)
	if !ok {
		return false
	}
	if summonItem.SummonType != summonItemTypeDecorative &&
		summonItem.SummonType != summonItemTypePet &&
		summonItem.SummonType != summonItemTypeWyvern {
		return false
	}

	// The gates below are shared by every summon kind, in the specified
	// order, ahead of the per-kind branches. Only a settled seat refuses: a
	// sit-down or stand-up still in progress passes, and holds a collar's
	// cast below.
	if live.Character.Seated() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotMoveWhileSitting))
		return true
	}
	// The observer, skills-disabled and casting rejections are specified
	// with no packet, and a use-item request leaves no client action
	// pending, so silence is the matching answer.
	if live.Character.ObserverMode() {
		return true
	}
	// restoringSummon extends the casting gate over a pets-row read whose
	// cast was already ended — by the hold ceiling, crowd control or death:
	// the summoner counts as still in that cast until its pet lands. It is
	// not part of the summon-slot check below, which would answer
	// SUMMON_ONLY_ONE where the specified answer is silence.
	if live.Character.AllSkillsDisabled() || live.Character.CastingNow() || l.restoringSummon(live) {
		return true
	}
	if summonItem.SummonType != summonItemTypeDecorative && (live.Character.MountType() != 0 || l.hasActiveSummon(live)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
		return true
	}
	// Only a swing in flight refuses: the attack stance that outlives it
	// does not.
	if live.attack != nil && live.attack.AttackingNow() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouCannotSummonInCombat))
		return true
	}
	if live.InBoat() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotCallPetFromThisLocation))
		return true
	}

	switch summonItem.SummonType {
	case summonItemTypeDecorative:
		return l.useDecorativeSummonItem(live, inv, inst, summonItem)
	case summonItemTypeWyvern:
		live.move.Stop()
		l.mountWyvern(live, summonItem.NPCID, inst.ObjectID)
		return true
	}

	def, ok := l.skills.Definition(summonCreatureSkillRef)
	if !ok {
		// SUMMON_CREATURE isn't loaded — a server-data gap, not a normal
		// rejection. Fall through unhandled so the caller's equip-toggle
		// fallback still answers the client, matching useItemAICast's own
		// unresolved-skill fallback.
		return false
	}

	// SUMMON_A_PET follows every attempt, refused, held or started: after
	// the refusal's answer, after the held cast's ActionFailed, or after the
	// cast-start packets.
	var run func()
	switch {
	case !l.attemptItemAICast(live, live.Character, def):
		// The attempt gate answers its refusal before the cast is held or
		// started.
	case itemAICastBusy(live):
		// A sit-down or stand-up in progress holds the cast as the next
		// CAST intention; it runs when the transition settles.
		live.deferSummonCreatureCast(inv, inst, def)
		sendMagicActionFailed(live)
	default:
		run = l.beginSummonCreatureCast(live, inv, inst, def)
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonAPet))
	if run != nil {
		run()
	}
	return true
}

// errCollarGone refuses a collar's cast once the collar has left the
// caster's inventory.
var errCollarGone = errors.New("summon: collar no longer in inventory")

// beginSummonCreatureCast starts collar's SUMMON_CREATURE cast on live
// itself, answering a refusal, and returns the cast's Schedule, or nil when
// it was refused. A collar that left inv since it was used refuses the cast
// with NOT_ENOUGH_ITEMS.
func (l *GameClientLink) beginSummonCreatureCast(live *livePlayer, inv *itemcontainer.Inventory, collar *item.Instance, def skillref.Definition) func() {
	controller := l.castController(live)
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Controller:  controller,
		Caster:      live.Character,
		Selected:    live.Character,
		Skill:       summonCreatureSkillRef,
		Definitions: l.skills,
		Hooks: actorcast.StartHooks{
			StopMovement: l.stopMovementForCast(live),
			AfterCanCast: func() error {
				if inv.ItemByObjectID(collar.ObjectID) == nil {
					return errCollarGone
				}
				return nil
			},
		},
	})
	if err != nil {
		// A refusal at the cost and condition checks names its reason
		// alone; one at the attempt gate also releases the client's action.
		switch {
		case errors.Is(err, errCollarGone):
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		case started.CanCastFailure && magicCastFailureReasonOnly(err):
			sendMagicCastFailureReason(live, started.Definition, err)
		default:
			sendMagicCastFailure(live, started.Definition, err)
		}
		return nil
	}
	target := started.Target
	plan := started.Plan

	casterObject := skillCastObject(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(
			casterObject,
			casterObject,
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
	sendSkillItemCharge(live, def, plan.ItemCharge)

	targetIDs := []int32{target.ObjectID()}
	return func() {
		controller.Schedule(plan, actorcast.Hooks{
			Launch: func() bool {
				l.broadcastLiveFrame(live, func() wire.Frame {
					return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), int32(def.ID), int32(def.Level), targetIDs)
				})
				return true
			},
			Hit: func() {
				// SUMMON_CREATURE's own handler (handler/skill/summon.go)
				// resolves the item back to a pet template and spawns it —
				// ApplyEffectsResult drives that the same way it drives every
				// other skill's Hit-phase effects.
				result := actorcast.ApplyItemEffectsResult(l.castEffects(), live.Character, target, def, collar)
				l.sendSkillHandlerResult(live, result)
				l.syncCubicTargets(live, result, def)
			},
			Failed: func(err error) {
				sendMagicCastFailureReason(live, def, err)
			},
		})
	}
}

func (l *GameClientLink) useDecorativeSummonItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, summonItem item.SummonItem) bool {
	if l.world == nil || l.ids == nil || l.npcs == nil {
		return false
	}
	template, ok := l.npcs.Get(int(summonItem.NPCID))
	if !ok {
		return false
	}
	var duplicate *npc.Decoration
	l.world.ForEachKnownInRadius(live, decorativeSummonRadius, func(obj world.Tracked) {
		if decoration, ok := obj.(*npc.Decoration); ok && decoration.Instance.Kind == npc.InstanceKind("ChristmasTree") && duplicate == nil {
			duplicate = decoration
		}
	})
	if duplicate != nil {
		live.SendFrame(serverpackets.FrameSystemMessageString(systemMessageCannotSummonAgain, duplicate.Name()))
		return true
	}
	if inv.DestroyItem(inst, 1) == nil {
		return true
	}
	live.move.Stop()
	x, y, z := live.Position()
	if !l.placeDecoration(template, live.Character.Name, x, y, z, live.Heading()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
	}
	return true
}

// placeDecoration places a decoration NPC of template, titled title, at
// (x, y) on the ground height below z, facing heading. It reports false,
// placing nothing, when the NPC cannot be built.
func (l *GameClientLink) placeDecoration(template *npc.Template, title string, x, y, z, heading int) bool {
	if l.geo != nil {
		z = int(l.geo.Height(x, y, z))
	}
	objectID, err := l.ids.NextID()
	if err != nil {
		return false
	}
	instance, err := npc.NewInstance(objectID, template)
	if err != nil {
		return false
	}
	decoration, err := npc.NewDecoration(instance, title, l.skills)
	if err != nil {
		return false
	}
	l.world.Spawn(decoration, x, y, z, heading)
	return true
}

// mountWyvern puts live on the wyvern npcID called by its collar
// controlItemID. Both hands are emptied first; a weapon that cannot be taken
// off refuses the mount with no packet, as specified, and a
// use-item request leaves no client action pending. The rider is then
// forced to run and loses its toggles before the wyvern's skill list, the
// Ride, the speed refresh and the feed gauge go out.
func (l *GameClientLink) mountWyvern(live *livePlayer, npcID, controlItemID int32) {
	if !l.disarm(live, true) {
		return
	}
	l.changeLiveMoveType(live, true)
	live.Character.EffectList().StopAllToggles()
	if !live.Character.Mount(npcID, controlItemID) {
		return
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameRide(live.ObjectID(), npcID)
	})
	l.broadcastCharacterInfo(live)
	live.Character.StartMountFeed()
}

// disarm takes live's weapon off, and its shield too when leftHandIncluded,
// naming each item removed, and refreshes its appearance for everyone
// around. It refuses, changing nothing, while a cursed weapon is held.
// Otherwise the attack in progress stops first, as a player's attack stop
// does: the character goes idle, dropping any attack intention it was still
// approaching, and the client is released with ActionFailed. Each hand that
// comes off refreshes the grade penalty ahead of its disarm message, as a
// player's body-slot unequip does.
func (l *GameClientLink) disarm(live *livePlayer, leftHandIncluded bool) bool {
	if live.Character.CursedWeaponEquipped() {
		return false
	}
	if live.attack != nil {
		live.attack.Stop()
	}
	live.tryToIdle(false)
	live.SendFrame(serverpackets.FrameActionFailed())
	slots := []item.Slot{item.SlotRHand}
	if leftHandIncluded {
		slots = append(slots, item.SlotLHand)
	}
	if inv := live.Inventory(); inv != nil && l.inventory != nil {
		for _, slot := range slots {
			res, ok := l.inventory.UnequipBodySlot(inv, int32(slot))
			if !ok || len(res.Changed) == 0 {
				continue
			}
			l.applyEquipStatChanges(live, inv, res)
			removed := res.Changed[0].Snapshot()
			sendUnequippedMessage(live, removed.TemplateID, removed.EnchantLevel)
		}
	}
	l.broadcastCharacterInfo(live)
	return true
}
