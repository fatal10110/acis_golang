package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	skillref "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// SummonItemsHandler is the etc-item handler name a pet-collar's
// <item handler="SummonItems"> attribute carries (SummonItems.java).
const SummonItemsHandler = "SummonItems"

// summonItemTypePet is SummonItemData's summonType value for "summon pet
// through an item" (SummonItems.java case 1). Types 0 (static decorative
// spawn, e.g. Christmas tree) and 2 (wyvern mount) are a different cast
// shape entirely and are not this handler's concern.
const summonItemTypePet = 1

// summonCreatureSkillRef is the hardcoded SUMMON_CREATURE skill id/level
// SummonItems.java always casts for a pet-collar item
// (SkillTable.getInstance().getInfo(2046, 1)), independent of which collar
// was used — the collar only selects the npc template via SummonItemData.
var summonCreatureSkillRef = skillref.Ref{ID: 2046, Level: 1}

// useSummonItem triggers the SUMMON_CREATURE cast for a pet-collar item
// (SummonItems.java case 1), driving it through the same timed
// Launch/Hit/Finish cast sequence useItemAICast uses for any other
// item-carried skill. Unlike a consumable, the collar itself is never
// destroyed — it stays the pet's persistent identity, matching
// SummonCreature.java's own lookup by control-item object id.
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
	if !ok || summonItem.SummonType != summonItemTypePet {
		return false
	}

	// SummonItems.java:37 (isAllSkillsDisabled/castingNow) precondition —
	// a silent drop, matching the reference's bare return. isSitting,
	// isInObserverMode, isAttackingNow and isInBoat are the same
	// reference method's other guards; this port has no sitting/observer/
	// attack-state/boat model yet for a player to be summoning from, so
	// those states cannot currently occur here.
	if live.AllSkillsDisabled() || live.CastingNow() {
		return true
	}

	def, ok := l.skills.Definition(summonCreatureSkillRef)
	if !ok {
		return true
	}

	// SetSummonSpawner is wired here rather than at world-enter time
	// (mirroring castController's own lazy-build-on-first-use), since no
	// other production path needs a spawner attached before a pet-collar
	// item is actually used.
	live.Character.SetSummonSpawner(&gameSummonSpawner{link: l, live: live})

	controller := l.castController(live)
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Now:         time.Now(),
		Controller:  controller,
		Caster:      live.Character,
		Selected:    live.Character,
		Skill:       summonCreatureSkillRef,
		Definitions: l.skills,
	})
	if err != nil {
		sendMagicCastFailure(live, started.Definition, err)
		return true
	}
	target := started.Target
	plan := started.Plan

	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonAPet))

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

	targetIDs := []int32{target.ObjectID()}
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
			result := actorcast.ApplyItemEffectsResult(actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers}, live.Character, target, def, inst)
			sendSkillHandlerResult(live, result)
		},
		Failed: func(err error) {
			sendMagicCastFailureReason(live, def, err)
		},
	})
	return true
}
