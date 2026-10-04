package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Extended opcodes of the fishing packets.
const (
	OpcodeExFishingStart       uint16 = 0x0013
	OpcodeExFishingEnd         uint16 = 0x0014
	OpcodeExFishingStartCombat uint16 = 0x0015
	OpcodeExFishingHpRegen     uint16 = 0x0016
)

// Fishing system messages.
const (
	SystemMessageGotABite                       = 1449
	SystemMessageFishSpitTheHook                = 1450
	SystemMessageBaitStolenByFish               = 1451
	SystemMessageBaitLostFishGotAway            = 1452
	SystemMessageFishingPoleNotEquipped         = 1453
	SystemMessageBaitOnHookBeforeFishing        = 1454
	SystemMessageCannotFishUnderWater           = 1455
	SystemMessageCannotFishOnBoat               = 1456
	SystemMessageCannotFishHere                 = 1457
	SystemMessageFishingAttemptCancelled        = 1458
	SystemMessageNotEnoughBait                  = 1459
	SystemMessageReelLineAndStopFishing         = 1460
	SystemMessageCastLineAndStartFishing        = 1461
	SystemMessageCanUsePumpingOnlyWhileFishing  = 1462
	SystemMessageCanUseReelingOnlyWhileFishing  = 1463
	SystemMessageFishResistedAttemptToBringItIn = 1464
	SystemMessagePumpingSuccessfulS1Damage      = 1465 // number parameter
	SystemMessageFishResistedPumpingS1HP        = 1466 // number parameter
	SystemMessageReelingSuccessfulS1Damage      = 1467 // number parameter
	SystemMessageFishResistedReelingS1HP        = 1468 // number parameter
	SystemMessageYouCaughtSomething             = 1469
	SystemMessageWrongFishingShotGrade          = 1479
	SystemMessageCannotFishWhileUsingRecipeBook = 1638
	SystemMessageCaughtSomethingSmelly          = 1655
	SystemMessageReelingPumping3LevelsHigher    = 1670
	SystemMessageReelingSuccessfulPenaltyS1     = 1671 // number parameter
	SystemMessagePumpingSuccessfulPenaltyS1     = 1672 // number parameter
)

// FrameExFishingStart shows objectID casting its line to bait, fishing for
// a fish of fishType (-1 hides it). nightLure marks a night lure;
// rankingButton shows the fishing championship ranking button.
func FrameExFishingStart(objectID int32, fishType int32, bait location.Location, nightLure, rankingButton bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExFishingStart)
	w.WriteInt32(objectID)
	w.WriteInt32(fishType)
	w.WriteInt32(int32(bait.X))
	w.WriteInt32(int32(bait.Y))
	w.WriteInt32(int32(bait.Z))
	w.WriteUint8(boolUint8(nightLure))
	w.WriteUint8(boolUint8(rankingButton))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExFishingEnd shows objectID taking its line out of the water, win
// when it caught something.
func FrameExFishingEnd(objectID int32, win bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExFishingEnd)
	w.WriteInt32(objectID)
	w.WriteUint8(boolUint8(win))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExFishingStartCombat opens objectID's fight with a hooked fish: time
// left in seconds, the fish's HP, its mode (0 resting, 1 fighting), the
// lure kind (0 beginner, 1 normal, 2 night) and whether it deceives.
func FrameExFishingStartCombat(objectID, time, hp int32, mode, lureType, deceptive uint8) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExFishingStartCombat)
	w.WriteInt32(objectID)
	w.WriteInt32(time)
	w.WriteInt32(hp)
	w.WriteUint8(mode)
	w.WriteUint8(lureType)
	w.WriteUint8(deceptive)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FishingHPRegen is one fight gauge update.
type FishingHPRegen struct {
	Time, HP  int32
	Mode      uint8 // 0 HP stops, 1 HP rises
	GoodUse   uint8 // 0 none, 1 success, 2 failure
	Anim      uint8 // 0 none, 1 pumping, 2 reeling
	Penalty   int32
	Deceptive uint8 // 0 normal HP bar, 1 purple HP bar
}

// FrameExFishingHpRegen updates objectID's fight gauge.
func FrameExFishingHpRegen(objectID int32, g FishingHPRegen) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExFishingHpRegen)
	w.WriteInt32(objectID)
	w.WriteInt32(g.Time)
	w.WriteInt32(g.HP)
	w.WriteUint8(g.Mode)
	w.WriteUint8(g.GoodUse)
	w.WriteUint8(g.Anim)
	w.WriteInt32(g.Penalty)
	w.WriteUint8(g.Deceptive)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
