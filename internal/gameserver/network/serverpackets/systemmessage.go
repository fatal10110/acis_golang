package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Static system message ids used by focused packet helpers.
const (
	SystemMessageCannotMoveWhileSitting            = 31
	SystemMessageNotEnoughHP                       = 23
	SystemMessageNotEnoughMP                       = 24
	SystemMessageRejuvenatingHP                    = 25
	SystemMessageWelcomeToLineage                  = 34
	SystemMessageDeathPenaltyLevelS1Added          = 1916
	SystemMessageDeathPenaltyLifted                = 1917
	SystemMessageUseS1                             = 46
	SystemMessageS1PreparedForReuse                = 48
	SystemMessageEarnedS2S1S                       = 53
	SystemMessageNothingHappened                   = 61
	SystemMessageS1SuccessfullyEnchanted           = 62
	SystemMessageS1S2SuccessfullyEnchanted         = 63
	SystemMessageEnchantmentFailedS1Evaporated     = 64
	SystemMessageEnchantmentFailedS1S2Evaporated   = 65
	SystemMessageTargetTooFar                      = 22
	SystemMessageCastingInterrupted                = 27
	SystemMessageFailedToPickupAdena               = 55
	SystemMessageFailedToPickupS1                  = 56
	SystemMessageFailedToPickupS2S1S               = 57
	SystemMessageS1CannotBeUsed                    = 113
	SystemMessageS1Disarmed                        = 417
	SystemMessageEquipmentS1S2Removed              = 1064
	SystemMessageRequestS1ForTrade                 = 118
	SystemMessageS1DeniedTradeRequest              = 119
	SystemMessageBeginTradeWithS1                  = 120
	SystemMessageS1ConfirmedTrade                  = 121
	SystemMessageCannotAdjustItemsAfterConfirm     = 122
	SystemMessageTradeSuccessful                   = 123
	SystemMessageS1CanceledTrade                   = 124
	SystemMessageSlotsFull                         = 129
	SystemMessageOnceTradeConfirmedCannotMove      = 141
	SystemMessageAlreadyTrading                    = 142
	SystemMessageTargetIncorrect                   = 144
	SystemMessageTargetNotFound                    = 145
	SystemMessageCannotUseQuestItems               = 148
	SystemMessageCannotPickupOrUseItemTrading      = 149
	SystemMessageS1IsBusyTryLater                  = 153
	SystemMessageAttackFailed                      = 158
	SystemMessageS1ResistedYourS2                  = 139
	SystemMessageS1PerformingCounterattack         = 1997
	SystemMessageCounteredS1Attack                 = 1998
	SystemMessageS1DodgesAttack                    = 1999
	SystemMessageAvoidedS1Attack                   = 42
	SystemMessageLethalStrike                      = 1667
	SystemMessageLethalStrikeSuccessful            = 1668
	SystemMessageInvalidTarget                     = 109
	SystemMessageCannotDiscardDistanceTooFar       = 151
	SystemMessageItemMissingToLearnSkill           = 276
	SystemMessageLearnedSkill                      = 277
	SystemMessageNotEnoughSPToLearnSkill           = 278
	SystemMessageSelectItemToEnchant               = 303
	SystemMessageForceIncreasedToS1                = 323
	SystemMessageForceMaxLevelReached              = 324
	SystemMessageNotEnoughItems                    = 351
	SystemMessageInappropriateEnchantCondition     = 355
	SystemMessageEnchantScrollCancelled            = 423
	SystemMessageWeightLimitExceeded               = 422
	SystemMessageCrystallizeLevelTooLow            = 562
	SystemMessageCubicSummoningFailed              = 568
	SystemMessagePetCannotSentBackDuringBattle     = 579
	SystemMessageDeadPetCannotBeReturned           = 589
	SystemMessageCannotGiveItemsToDeadPet          = 590
	SystemMessageYouCannotRestoreHungryPets        = 594
	SystemMessageItemNotForPets                    = 544
	SystemMessagePetCannotCarryMoreItems           = 545
	SystemMessagePetTooEncumbered                  = 546
	SystemMessageSummonAPet                        = 547
	SystemMessageSummonOnlyOne                     = 580
	SystemMessageYouCannotSummonInCombat           = 578
	SystemMessageNotCallPetFromThisLocation        = 604
	SystemMessageNoMoreSkillsToLearn               = 750
	SystemMessagePetCannotUseItem                  = 972
	SystemMessagePetPutOnS1                        = 1024
	SystemMessagePetTookOffS1                      = 1025
	SystemMessageItemCrystallized                  = 1258
	SystemMessageUseOfItemWillBeAuto               = 1433
	SystemMessageAutoUseOfItemCancelled            = 1434
	SystemMessageCannotDoWhileFishing              = 1471
	SystemMessageItemCantBeEquippedForOlympiad     = 1507
	SystemMessageItemUnavailableForOlympiad        = 1508
	SystemMessageBlessedEnchantFailed              = 1517
	SystemMessageAttentionS1PickedUpS2             = 1533
	SystemMessageAttentionS1PickedUpS2S3           = 1534
	SystemMessageItemsUnavailableForStore          = 1578
	SystemMessageNoServitorCannotAutomateUse       = 1676
	SystemMessageCannotEnchantWhileStore           = 1688
	SystemMessageExchangeHasEnded                  = 1266
	SystemMessagePetRefusingOrder                  = 1864
	SystemMessagePetTooHighToControl               = 1918
	SystemMessageS1                                = 1987
	SystemMessageThereIsNoSkillThatEnablesEnchant  = 1438
	SystemMessageMissingItemsToEnchantSkill        = 1439
	SystemMessageSucceededEnchantingSkillS1        = 1440
	SystemMessageFailedEnchantingSkillS1           = 1441
	SystemMessageNotEnoughSPToEnchantSkill         = 1443
	SystemMessageNotEnoughExpToEnchantSkill        = 1444
	SystemMessageSoulshotsGradeMismatch            = 337
	SystemMessageNotEnoughSoulshots                = 338
	SystemMessageCannotUseSoulshots                = 339
	SystemMessageEnabledSoulshot                   = 342
	SystemMessageSpiritshotsGradeMismatch          = 530
	SystemMessageNotEnoughSpiritshots              = 531
	SystemMessageCannotUseSpiritshots              = 532
	SystemMessageEnabledSpiritshot                 = 533
	SystemMessagePetsNotAvailableAtThisTime        = 574
	SystemMessagePetUsesS1                         = 1018
	SystemMessagePetReceivedS2DamageByS1           = 1016
	SystemMessageSummonReceivedS2ByS1              = 1027
	SystemMessageShotsNotAvailableForDeadPet       = 1598
	SystemMessageNotEnoughSpiritshotsForPet        = 1700
	SystemMessageNotEnoughSoulshotsForPet          = 1701
	SystemMessageCantAtkPeacezone                  = 84
	SystemMessageTargetInPeacezone                 = 85
	SystemMessageCantSeeTarget                     = 181
	SystemMessageNamingCharnameUpTo16Chars         = 80
	SystemMessageNamingAlreadyInUseByAnotherPet    = 584
	SystemMessageNamingPetnameContainsInvalidChars = 591
	SystemMessageNamingYouCannotSetNameOfThePet    = 695
	SystemMessageDistTooFarCastingStopped          = 748

	// Experience, SP and level change feedback.
	SystemMessageEarnedS1Experience     = 45   // number parameter
	SystemMessageYouEarnedS1ExpAndS2SP  = 95   // two number parameters
	SystemMessageYouIncreasedYourLevel  = 96   // no parameter
	SystemMessageAcquiredS1SP           = 331  // number parameter
	SystemMessageSPDecreasedS1          = 538  // number parameter
	SystemMessageExpDecreasedByS1       = 539  // number parameter
	SystemMessageDrownDamage            = 297  // number parameter
	SystemMessageSkillRemovedDueLackHP  = 610  // no parameter
	SystemMessageSkillRemovedDueLackMP  = 140  // no parameter
	SystemMessageRemainingMana10Minutes = 1979 // item-name parameter
	SystemMessageRemainingMana5Minutes  = 1980 // item-name parameter
	SystemMessageRemainingMana1Minute   = 1981 // item-name parameter
	SystemMessageRemainingManaIsNow0    = 1982 // item-name parameter

	// Karma change feedback.
	SystemMessageYourKarmaHasBeenChangedToS1 = 1282 // number parameter

	// Periodic in-game clock messages.
	SystemMessagePlayingForLongTime       = 764  // no parameter
	SystemMessageNightSkillEffectApplies  = 1131 // skill-name parameter
	SystemMessageDaySkillEffectDisappears = 1132 // skill-name parameter

	// Shield-block feedback.
	SystemMessageShieldDefenceSuccessful       = 111  // no parameter
	SystemMessageExcellentShieldDefenseSuccess = 1281 // no parameter
)

// SystemMessage parameter types used by focused packet helpers.
const (
	SystemMessageParamText       = 0
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

// FrameSystemMessageString builds a SystemMessage packet with one text
// parameter.
func FrameSystemMessageString(id int, text string) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(1)
	w.WriteInt32(SystemMessageParamText)
	w.WriteString(text)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageStringItemName builds a SystemMessage packet with text
// followed by an item-name parameter.
func FrameSystemMessageStringItemName(id int, text string, itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(2)
	w.WriteInt32(SystemMessageParamText)
	w.WriteString(text)
	w.WriteInt32(SystemMessageParamItemName)
	w.WriteInt32(itemID)
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

// FrameSystemMessageNumber builds a SystemMessage packet with one number
// parameter.
func FrameSystemMessageNumber(id int, number int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(1)
	w.WriteInt32(SystemMessageParamNumber)
	w.WriteInt32(number)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageStringNumber builds a SystemMessage packet with text and number parameters.
func FrameSystemMessageStringNumber(id int, text string, number int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(2)
	w.WriteInt32(SystemMessageParamText)
	w.WriteString(text)
	w.WriteInt32(SystemMessageParamNumber)
	w.WriteInt32(number)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSystemMessageTwoNumbers builds a SystemMessage packet with two number
// parameters.
func FrameSystemMessageTwoNumbers(id int, first, second int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(2)
	w.WriteInt32(SystemMessageParamNumber)
	w.WriteInt32(first)
	w.WriteInt32(SystemMessageParamNumber)
	w.WriteInt32(second)
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

// FrameSystemMessageStringNumberItemName builds a SystemMessage packet with
// text, number, and item-name parameters.
func FrameSystemMessageStringNumberItemName(id int, text string, number int32, itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(3)
	w.WriteInt32(SystemMessageParamText)
	w.WriteString(text)
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

// FrameSystemMessageStringSkillName builds a SystemMessage with text followed
// by a skill-name parameter.
func FrameSystemMessageStringSkillName(id int, text string, skillID, level int32) wire.Frame {
	w := newFrameWriter(OpcodeSystemMessage)
	w.WriteInt32(int32(id))
	w.WriteInt32(2)
	w.WriteInt32(SystemMessageParamText)
	w.WriteString(text)
	w.WriteInt32(SystemMessageParamSkillName)
	w.WriteInt32(skillID)
	w.WriteInt32(level)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
