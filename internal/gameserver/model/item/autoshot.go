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
