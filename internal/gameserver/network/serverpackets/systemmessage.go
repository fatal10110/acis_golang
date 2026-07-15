package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Static system message ids used by focused packet helpers.
const (
	SystemMessageNotEnoughHP                     = 23
	SystemMessageNotEnoughMP                     = 24
	SystemMessageWelcomeToLineage                = 34
	SystemMessageUseS1                           = 46
	SystemMessageS1PreparedForReuse              = 48
	SystemMessageEarnedS2S1S                     = 53
	SystemMessageNothingHappened                 = 61
	SystemMessageS1SuccessfullyEnchanted         = 62
	SystemMessageS1S2SuccessfullyEnchanted       = 63
	SystemMessageEnchantmentFailedS1Evaporated   = 64
	SystemMessageEnchantmentFailedS1S2Evaporated = 65
	SystemMessageTargetTooFar                    = 22
	SystemMessageS1CannotBeUsed                  = 113
	SystemMessageAlreadyTrading                  = 142
	SystemMessageCannotPickupOrUseItemTrading    = 149
	SystemMessageInvalidTarget                   = 109
	SystemMessageCannotDiscardDistanceTooFar     = 151
	SystemMessageItemMissingToLearnSkill         = 276
	SystemMessageLearnedSkill                    = 277
	SystemMessageNotEnoughSPToLearnSkill         = 278
	SystemMessageSelectItemToEnchant             = 303
	SystemMessageNotEnoughItems                  = 351
	SystemMessageInappropriateEnchantCondition   = 355
	SystemMessageEnchantScrollCancelled          = 423
	SystemMessageCrystallizeLevelTooLow          = 562
	SystemMessagePetCannotSentBackDuringBattle   = 579
	SystemMessageDeadPetCannotBeReturned         = 589
	SystemMessageCannotGiveItemsToDeadPet        = 590
	SystemMessageYouCannotRestoreHungryPets      = 594
	SystemMessageItemNotForPets                  = 544
	SystemMessagePetCannotCarryMoreItems         = 545
	SystemMessagePetTooEncumbered                = 546
	SystemMessageNoMoreSkillsToLearn             = 750
	SystemMessagePetCannotUseItem                = 972
	SystemMessagePetPutOnS1                      = 1024
	SystemMessagePetTookOffS1                    = 1025
	SystemMessageItemCrystallized                = 1258
	SystemMessageUseOfItemWillBeAuto             = 1433
	SystemMessageAutoUseOfItemCancelled          = 1434
	SystemMessageBlessedEnchantFailed            = 1517
	SystemMessageNoServitorCannotAutomateUse     = 1676
	SystemMessageCannotEnchantWhileStore         = 1688
	SystemMessagePetRefusingOrder                = 1864
	SystemMessagePetTooHighToControl             = 1918
)

// SystemMessage parameter types used by focused packet helpers.
const (
	SystemMessageParamNumber     = 1
	SystemMessageParamItemName   = 3
	SystemMessageParamSkillName  = 4
	SystemMessageParamItemNumber = 6
)

// OpcodeSystemMessage is the wire opcode for a system message.
const OpcodeSystemMessage = 0x64

// FrameSystemMessage builds a static no-parameter SystemMessage packet.
func FrameSystemMessage(id int) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageItemName builds a SystemMessage packet with one item-name
// parameter.
func FrameSystemMessageItemName(id int, itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(1)
	w.WriteInt32(SystemMessageParamItemName)
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageNumberItemName builds a SystemMessage packet with a
// number parameter followed by an item-name parameter.
func FrameSystemMessageNumberItemName(id int, number int32, itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(2)
	w.WriteInt32(SystemMessageParamNumber)
	w.WriteInt32(number)
	w.WriteInt32(SystemMessageParamItemName)
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageItemNameItemNumber builds a SystemMessage packet with
// an item-name parameter followed by an item-number parameter.
func FrameSystemMessageItemNameItemNumber(id int, itemID int32, count int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(2)
	w.WriteInt32(SystemMessageParamItemName)
	w.WriteInt32(itemID)
	w.WriteInt32(SystemMessageParamItemNumber)
	w.WriteInt32(count)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageSkillName builds a SystemMessage packet with one skill-name
// parameter.
func FrameSystemMessageSkillName(id int, skillID, level int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(1)
	w.WriteInt32(SystemMessageParamSkillName)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
