package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// FrameExShowVariationMakeWindow opens the augmentation UI.
func FrameExShowVariationMakeWindow() wire.Frame {
	return frameExtendedOnly(OpcodeExShowVariationMakeWindow)
}

// FrameExShowVariationCancelWindow opens the augmentation removal UI.
func FrameExShowVariationCancelWindow() wire.Frame {
	return frameExtendedOnly(OpcodeExShowVariationCancelWindow)
}

// FrameExConfirmVariationItem confirms the selected augmentation target.
func FrameExConfirmVariationItem(objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExConfirmVariationItem)
	w.WriteInt32(objectID)
	w.WriteInt32(1)
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExConfirmVariationRefiner confirms the selected life stone and the
// gemstone cost required by the target item grade.
func FrameExConfirmVariationRefiner(refinerObjectID, lifeStoneItemID, gemstoneItemID, gemstoneCount int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExConfirmVariationRefiner)
	w.WriteInt32(refinerObjectID)
	w.WriteInt32(lifeStoneItemID)
	w.WriteInt32(gemstoneItemID)
	w.WriteInt32(gemstoneCount)
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExConfirmVariationGemstone confirms the selected gemstone stack.
func FrameExConfirmVariationGemstone(gemstoneObjectID, count int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExConfirmVariationGemstone)
	w.WriteInt32(gemstoneObjectID)
	w.WriteInt32(1)
	w.WriteInt32(count)
	w.WriteInt32(1)
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExConfirmCancelItem confirms the item and fee for augmentation
// removal.
func FrameExConfirmCancelItem(objectID, itemID, augmentationID int32, price int64) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExConfirmCancelItem)
	w.WriteInt32(objectID)
	w.WriteInt32(itemID)
	low, high := splitAugmentationID(augmentationID)
	w.WriteInt32(low)
	w.WriteInt32(high)
	w.WriteInt64(price)
	w.WriteInt32(1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExVariationResult reports the result of applying augmentation.
func FrameExVariationResult(stat12, stat34, result int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExVariationResult)
	w.WriteInt32(stat12)
	w.WriteInt32(stat34)
	w.WriteInt32(result)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExVariationResultFailed reports a failed augmentation attempt.
func FrameExVariationResultFailed() wire.Frame {
	return FrameExVariationResult(0, 0, 0)
}

// FrameExVariationCancelResult reports the result of removing augmentation.
func FrameExVariationCancelResult(result int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExVariationCancelResult)
	w.WriteInt32(1)
	w.WriteInt32(result)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func frameExtendedOnly(opcode uint16) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(opcode)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func splitAugmentationID(id int32) (low int32, high int32) {
	return int32(int16(id)), id >> 16
}
