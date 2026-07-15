package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodeChooseInventoryItem opens the client-side enchant item picker
	// for the scroll item id.
	OpcodeChooseInventoryItem = 0x6f
	// OpcodeEnchantResult closes the enchant window with the result code.
	OpcodeEnchantResult = 0x81
)

// EnchantResult is the result code carried by EnchantResult.
type EnchantResult int32

const (
	EnchantResultSuccess            EnchantResult = 0
	EnchantResultBrokenWithCrystals EnchantResult = 1
	EnchantResultCancelled          EnchantResult = 2
	EnchantResultUnsuccess          EnchantResult = 3
	EnchantResultBrokenNoCrystals   EnchantResult = 4
)

// FrameChooseInventoryItem builds the ChooseInventoryItem packet for an
// enchant scroll item id.
func FrameChooseInventoryItem(itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeChooseInventoryItem)
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameEnchantResult builds the EnchantResult packet for result.
func FrameEnchantResult(result EnchantResult) wire.Frame {
	w := newFrameWriter(OpcodeEnchantResult)
	w.WriteInt32(int32(result))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
