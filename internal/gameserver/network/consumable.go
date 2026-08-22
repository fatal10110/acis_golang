package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// useConsumableSkillItem runs the instant-cast (potion) item-use path and
// maps its outcome to client packets: InventoryUpdate for the consumed
// stack, broadcast MagicSkillUse, USE_S1, and a StatusUpdate when the
// skill changed HP or MP. A reuse rejection or consume failure maps to
// the same cast-failure reply a player skill cast produces.
//
// It reports whether inst was handled by this path. A non-consumable, an
// item whose carried skill isn't an instant-cast potion, or an herb
// returns false so the caller's equip-toggle fallback still answers the
// client.
func (l *GameClientLink) useConsumableSkillItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	results := itemhandler.UseAll(itemhandler.UseRequest{
		Caster:      live.Character,
		Inventory:   inv,
		Item:        inst,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Destroyer:   l.inventory,
		Summon:      l.activeServitorTarget(live),
		Target:      live.Character.CurrentTarget(),
	})
	for _, res := range results {
		switch res.Outcome {
		case itemhandler.NotHandled:
			return false
		case itemhandler.PetRejected:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
			return true
		case itemhandler.ReuseRejected:
			sendMagicCastFailure(live, res.Skill, actorcast.ErrSkillDisabled)
			return true
		case itemhandler.NotEnoughItems:
			sendItemConsumeFailure(live)
			return true
		case itemhandler.ConditionRejected:
			sendItemSkillConditionFailure(live, res)
			return true
		case itemhandler.Applied:
			if res.SharedReuseGroup >= 0 {
				live.SendFrame(serverpackets.FrameExUseSharedGroupItem(inst.TemplateID, res.SharedReuseGroup, res.ReuseMillis, res.ReuseMillis))
			}
			self := skillCastObject(live)
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(self, self, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
			})
			if res.MirroredSummon != nil {
				summonObject := skillCastObject(res.MirroredSummon)
				l.broadcastLiveFrame(live, func() wire.Frame {
					return serverpackets.FrameMagicSkillUse(summonObject, summonObject, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
				})
			}
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageUseS1, int32(res.Skill.ID), int32(res.Skill.Level)))
			applyItemCastCharges(live, res)
			beforeVitals := live.Vitals()
			l.sendSkillHandlerResult(live, res.Apply())
			sendMagicStatusUpdate(live, beforeVitals)
			if res.HasShortBuff {
				live.Character.UpdateShortBuff(res.ShortBuffSkillID, res.ShortBuffLevel, res.ShortBuffDurationSeconds)
			}
		}
	}
	return true
}

// applyItemCastCharges applies the item-carried skill's Force/Soul charges
// to live, matching PlayableCast<Player>.doInstantCast's override
// (PlayerCast.java:108-114): charges run after the cast's own packets
// (shared-reuse, MagicSkillUse, USE_S1) and before the skill's effects
// apply. Only the player who used the item — never the pet/summon herb
// path (network/pet_herb.go) — reaches this, matching PlayableCast.java's
// base doInstantCast having no charge handling at all.
func applyItemCastCharges(live *livePlayer, res itemhandler.UseResult) {
	if res.Skill.NumCharges <= 0 {
		return
	}
	if res.Skill.MaxCharges > 0 {
		live.Character.IncreaseCharges(res.Skill.NumCharges, res.Skill.MaxCharges)
	} else {
		live.Character.DecreaseCharges(res.Skill.NumCharges)
	}
}

func sendItemSkillConditionFailure(live *livePlayer, res itemhandler.UseResult) {
	if live == nil || res.Condition.MessageID <= 0 {
		return
	}
	if res.Condition.AddName {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(int(res.Condition.MessageID), int32(res.Skill.ID), int32(res.Skill.Level)))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(int(res.Condition.MessageID)))
}
