package item

// IsFishingShotID reports whether itemID is one of the fishing shots that
// cannot be automated.
func IsFishingShotID(itemID int32) bool {
	return itemID >= 6535 && itemID <= 6540
}

// IsSummonShotID reports whether itemID is one of the servitor shot items.
func IsSummonShotID(itemID int32) bool {
	return itemID >= 6645 && itemID <= 6647
}

// SummonShotIDs lists the servitor shot items in ascending id order.
func SummonShotIDs() []int32 { return []int32{6645, 6646, 6647} }

// BeastSoulshotID is the servitor soulshot; the other two servitor shots
// are spiritshots.
const BeastSoulshotID int32 = 6645

// BlessedBeastSpiritshotID is the blessed servitor spiritshot, which cannot
// be automated during an Olympiad match.
const BlessedBeastSpiritshotID int32 = 6647

// IsBlessedSpiritshotID reports whether itemID is one of the graded
// blessed spiritshots, which cannot be automated during an Olympiad match.
func IsBlessedSpiritshotID(itemID int32) bool {
	return itemID >= 3947 && itemID <= 3952
}

// IsAutoSpiritshotID reports whether turning on automatic use of itemID
// with a mismatched weapon grade reports a spiritshot grade mismatch rather
// than a soulshot one: the graded and blessed spiritshots and the
// beginner's spiritshot.
func IsAutoSpiritshotID(itemID int32) bool {
	return (itemID >= 2509 && itemID <= 2514) || IsBlessedSpiritshotID(itemID) || itemID == 5790
}
