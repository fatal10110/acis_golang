package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// sendSummonOwnerMessage sends message to the player actor answers to, if
// that player is in the world.
func (l *GameClientLink) sendSummonOwnerMessage(actor *summon.Actor, message int) {
	if owner, ok := l.currentSummonOwner(actor); ok {
		owner.SendFrame(serverpackets.FrameSystemMessage(message))
	}
}

// autoFeedPet has a hungry pet eat one unit of the food item objectID from
// its own inventory, on its feed tick. Its owner is told the pet took the
// food, then the food's item handler runs with the pet as its user
// (consumePetFood). An item whose handler is not the pet-food one is left
// alone, with no message.
func (l *GameClientLink) autoFeedPet(actor *summon.Actor, objectID int32) {
	petInv := actor.PetInventory()
	if petInv == nil {
		return
	}
	inst := petInv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := petInv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != petFoodsHandler {
		return
	}
	owner, _ := l.currentSummonOwner(actor)
	if owner != nil {
		owner.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetTookS1BecauseHeWasHungry, inst.TemplateID))
	}
	l.consumePetFood(owner, actor, petInv, inst)
}
