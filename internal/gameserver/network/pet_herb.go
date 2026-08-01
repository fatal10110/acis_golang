package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) consumePetHerb(live *livePlayer, pet *summon.Actor, inv *itemcontainer.Inventory, herb *item.Instance) {
	res := itemhandler.Use(itemhandler.UseRequest{
		Caster:      pet,
		Inventory:   inv,
		Item:        herb,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Destroyer:   l.inventory,
		IsPet:       true,
	})
	switch res.Outcome {
	case itemhandler.PetRejected:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
		return
	case itemhandler.ReuseRejected:
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(res.Skill.ID), int32(res.Skill.Level)))
		return
	case itemhandler.Applied:
	default:
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	self := skillCastObject(pet)
	l.broadcastPetFrame(live, pet, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(self, self, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
	})
	res.Apply()
	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessagePetUsesS1, int32(res.Skill.ID), int32(res.Skill.Level)))
	l.broadcastPetFrame(live, pet, func() wire.Frame {
		return serverpackets.FrameStatusUpdate(pet.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusCurrentHP, Value: int(pet.HP())},
			{Type: serverpackets.StatusCurrentMP, Value: int(pet.MPValue())},
		})
	})
}

func (l *GameClientLink) broadcastPetFrame(live *livePlayer, pet *summon.Actor, frame func() wire.Frame) {
	if live == nil || pet == nil {
		return
	}
	broadcastFrame(frame, func(send func(frameReceiver)) {
		send(live)
		if l.world == nil {
			return
		}
		l.world.ForEachKnown(pet, func(o world.Tracked) {
			if o.ObjectID() == live.ObjectID() {
				return
			}
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}
