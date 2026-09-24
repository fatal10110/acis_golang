package network

import (
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
	if !ok {
		return false
	}
	if summonItem.SummonType == summonItemTypeDecorative {
		return l.useDecorativeSummonItem(live, inv, inst, summonItem)
	}
	if summonItem.SummonType == summonItemTypeWyvern {
		if !live.Character.Standing() {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotMoveWhileSitting))
			return true
		}
		// restoringSummon extends this gate over the rest of a pets-row
		// read whose cast the hold ceiling already ended: the reference
		// is still casting there and returns here silently
		// (SummonItems.java:36-37), before the summon-slot check below.
		if live.Character.AllSkillsDisabled() || live.Character.CastingNow() || l.restoringSummon(live) {
			return true
		}
		if live.Character.MountType() != 0 || l.hasActiveSummon(live) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
			return true
		}
		if live.Character.InCombat() {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouCannotSummonInCombat))
			return true
		}
		live.move.Stop()
		if !live.Character.Mount(summonItem.NPCID, inst.ObjectID) {
			return true
		}
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameRide(live.ObjectID(), summonItem.NPCID)
		})
		live.Character.UpdateUserInfo()
		return true
	}
	if summonItem.SummonType != summonItemTypePet {
		return false
	}
	if !live.Character.Standing() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotMoveWhileSitting))
		return true
	}
	// A pets-row read still in flight means the previous summon cast has
	// hit; the reference is still casting at that point and returns here
	// silently (SummonItems.java:37-38, above the switch at :64-65), so no
	// second cast starts. Without this, past the hold ceiling the cast is
	// over and StartItemSkill's already-casting rejection no longer fires,
	// so the collar would run a whole second cast — broadcasting
	// MagicSkillUse and MagicSkillLaunched — only to be rejected at
	// SpawnPet's gate when it hits.
	//
	// Only the restore window is gated here. The rest of what :37-38
	// covers for this branch (an ordinary cast in progress, disabled
	// skills) still reaches StartItemSkill and answers through
	// sendMagicCastFailure; that pre-existing divergence belongs to the
	// pre-cast gate #2369 adds.
	if l.restoringSummon(live) {
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

	// The spawner is created here rather than at world-enter time
	// (mirroring castController's own lazy-build-on-first-use), since no
	// other production path needs one before a pet-collar item is actually
	// used — but only on the first use, not every one: link/live never
	// change for the life of the connection.
	if live.summonSpawner.Load() == nil {
		live.summonSpawner.Store(&gameSummonSpawner{link: l, live: live})
	}

	controller := l.castController(live)
	started, err := actorcast.StartItemSkill(actorcast.ItemSkillRequest{
		Controller:  controller,
		Caster:      live.Character,
		Selected:    live.Character,
		Skill:       summonCreatureSkillRef,
		Definitions: l.skills,
		Hooks: actorcast.StartHooks{
			StopMovement: l.stopMovementForCast(live),
		},
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
			l.sendSkillHandlerResult(live, result)
			l.syncCubicTargets(live, result, def)
		},
		Failed: func(err error) {
			sendMagicCastFailureReason(live, def, err)
		},
	})
	return true
}

func (l *GameClientLink) useDecorativeSummonItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, summonItem item.SummonItem) bool {
	if !live.Character.Standing() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotMoveWhileSitting))
		return true
	}
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
	objectID, err := l.ids.NextID()
	if err != nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return true
	}
	instance, err := npc.NewInstance(objectID, template)
	if err != nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return true
	}
	decoration, err := npc.NewDecoration(instance, live.Character.Name)
	if err != nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return true
	}
	x, y, z := live.Position()
	l.world.Spawn(decoration, x, y, z, live.Heading())
	return true
}
