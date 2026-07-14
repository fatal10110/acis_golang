package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Static system message ids used by live summon command feedback.
const (
	SystemMessageNotEnoughHP                   = 23
	SystemMessageNotEnoughMP                   = 24
	SystemMessageUseS1                         = 46
	SystemMessageS1PreparedForReuse            = 48
	SystemMessageWelcomeToLineage              = 34
	SystemMessageNothingHappened               = 61
	SystemMessageInvalidTarget                 = 109
	SystemMessageCannotDiscardDistanceTooFar   = 151
	SystemMessageItemMissingToLearnSkill       = 276
	SystemMessageLearnedSkill                  = 277
	SystemMessageNotEnoughSPToLearnSkill       = 278
	SystemMessageNotEnoughItems                = 351
	SystemMessageCrystallizeLevelTooLow        = 562
	SystemMessageNoMoreSkillsToLearn           = 750
	SystemMessageItemCrystallized              = 1258
	SystemMessageUseOfItemWillBeAuto           = 1433
	SystemMessageAutoUseOfItemCancelled        = 1434
	SystemMessagePetCannotSentBackDuringBattle = 579
	SystemMessageDeadPetCannotBeReturned       = 589
	SystemMessageYouCannotRestoreHungryPets    = 594
	SystemMessageNoServitorCannotAutomateUse   = 1676
	SystemMessagePetRefusingOrder              = 1864
	SystemMessagePetTooHighToControl           = 1918
)

// SystemMessage parameter types used by focused packet helpers.
const (
	SystemMessageParamItemName  = 3
	SystemMessageParamSkillName = 4
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
