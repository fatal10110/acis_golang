package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// petFoodsHandler is the etc-item handler name RequestPetUseItem.java
// resolves a pet-food item to (PetFoods.java), distinct from the
// potion/skill items itemhandler.ItemSkillsHandler/ElixirsHandler cover.
const petFoodsHandler = "PetFoods"

// petFoodMagicIDs maps each pet-food item id to the feed skill
// PetFoods.java hardcodes for it (PetFoods.java:20-42): the skill's Feed
// value at level 1 is the meal-gauge amount restored, scaled by the
// configured pet food rate.
var petFoodMagicIDs = map[int32]int32{
	2515: 2048, // Wolf's food
	4038: 2063, // Hatchling's food
	5168: 2101, // Strider's food
	5169: 2102, // ClanHall / Castle Strider's food
	6316: 2180, // Wyvern's food
	7582: 2048, // Baby Pet's food
}

// usePetConsumable dispatches a non-equipment pet-inventory item that
// petitem.UseItem accepted as an eligible consumable (petitem.UseConsumable)
// to its etc-item handler, matching RequestPetUseItem.java's else branch:
// ItemHandler.getInstance().getHandler(item.getEtcItem()).useItem(pet, item, false).
func (l *GameClientLink) usePetConsumable(live *livePlayer, pet *summon.Actor, petInv *itemcontainer.Inventory, objectID int32) {
	inst := petInv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	tmpl, ok := petInv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.EtcItem == nil {
		return
	}
	switch tmpl.EtcItem.Handler {
	case itemhandler.ItemSkillsHandler, itemhandler.ElixirsHandler:
		l.consumePetPotion(live, pet, petInv, inst)
	case petFoodsHandler:
		l.consumePetFood(live, pet, petInv, inst)
	}
}

// consumePetPotion runs the ItemSkills instant-cast path for a pet-used
// potion, the RequestPetUseItem counterpart to consumePetHerb.
func (l *GameClientLink) consumePetPotion(live *livePlayer, pet *summon.Actor, petInv *itemcontainer.Inventory, inst *item.Instance) {
	results := itemhandler.UseAll(itemhandler.UseRequest{
		Caster:      pet,
		Inventory:   petInv,
		Item:        inst,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Destroyer:   l.inventory,
		IsPet:       true,
	})
	defer l.broadcastPetFrame(live, pet, func() wire.Frame {
		return serverpackets.FrameStatusUpdate(pet.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusCurrentHP, Value: int(pet.HP())},
			{Type: serverpackets.StatusCurrentMP, Value: int(pet.MPValue())},
		})
	})
	for _, res := range results {
		switch res.Outcome {
		case itemhandler.PetRejected:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemNotForPets))
			return
		case itemhandler.ReuseRejected:
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1PreparedForReuse, int32(res.Skill.ID), int32(res.Skill.Level)))
			return
		case itemhandler.ConditionRejected:
			sendItemSkillConditionFailure(live, res)
			return
		case itemhandler.Applied:
			self := skillCastObject(pet)
			l.broadcastPetFrame(live, pet, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(self, self, int32(res.Skill.ID), int32(res.Skill.Level), 0, 0, false)
			})
			l.sendSkillHandlerResult(live, res.Apply())
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessagePetUsesS1, int32(res.Skill.ID), int32(res.Skill.Level)))
		default: // NotHandled, NotEnoughItems: no reply, matching the absence of
			// a client-visible response for these outcomes on this path.
			return
		}
	}
}

// consumePetFood runs PetFoods.java's feed path for a pet-eaten food item:
// destroy one unit, broadcast the feed skill's visual effect, and raise the
// pet's meal gauge by the skill's Feed value scaled by the configured pet
// food rate, alerting the owner if the pet is still hungry afterward
// (PetFoods.java:49-68).
func (l *GameClientLink) consumePetFood(live *livePlayer, pet *summon.Actor, petInv *itemcontainer.Inventory, inst *item.Instance) {
	amount, ok := petFoodFeedAmount(l.skills, l.petConfig.FoodRate, inst.TemplateID)
	if !ok {
		return
	}
	magicID := petFoodMagicIDs[inst.TemplateID]
	if _, ok := l.inventory.DestroyItem(petInv, inst.ObjectID, 1); !ok {
		return
	}

	self := skillCastObject(pet)
	l.broadcastPetFrame(live, pet, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(self, self, magicID, 1, 0, 0, false)
	})

	_, stillHungry := pet.AddFed(amount)
	if stillHungry {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYourPetAteALittleButIsStillHungry))
	}
}

// petFoodFeedAmount resolves the meal-gauge amount a pet-food item template
// restores: PetFoods.java's hardcoded item->feed-skill map, scaled by the
// configured pet food rate (PetFoods.java:63: skill.getFeed() *
// Config.PET_FOOD_RATE). Used for both the manual "eat from inventory"
// packet and the auto-feed tick, since both dispatch through the same
// item handler in the reference.
func petFoodFeedAmount(skills *skillstate.Persistence, foodRate int, templateID int32) (int, bool) {
	magicID, ok := petFoodMagicIDs[templateID]
	if !ok {
		return 0, false
	}
	def, ok := skills.Definition(modelskill.Ref{ID: modelskill.ID(magicID), Level: 1})
	if !ok {
		return 0, false
	}
	return def.Feed * foodRate, true
}
