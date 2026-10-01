package item

// PaganKeysHandler is the etc-item handler name the Pagan Temple keys carry.
// A key opens its doors itself; the skill it carries is never cast.
const PaganKeysHandler = "PaganKeys"

// Pagan Temple keys and the doors each one fits.
const (
	keyOfSplendorRoom = 8056
	anteroomKey       = 8273
	keyOfDarkness     = 8275

	splendorRoomDoorA = 23150003
	splendorRoomDoorB = 23150004
)

// PaganKeyDoors returns the doors keyID opens when used on the door doorID.
// The Key of Splendor Room opens both splendor room doors from either one;
// the other keys open only the door used on. wrongDoor reports a known key
// used on a door it does not fit; an unknown key opens nothing and fits
// nothing, so both results are empty.
func PaganKeyDoors(keyID int32, doorID int) (doors []int, wrongDoor bool) {
	switch keyID {
	case keyOfSplendorRoom:
		if doorID == splendorRoomDoorA || doorID == splendorRoomDoorB {
			return []int{splendorRoomDoorA, splendorRoomDoorB}, false
		}
	case anteroomKey:
		if doorID >= 19160002 && doorID <= 19160009 {
			return []int{doorID}, false
		}
	case keyOfDarkness:
		if doorID == 19160012 || doorID == 19160013 {
			return []int{doorID}, false
		}
	default:
		return nil, false
	}
	return nil, true
}
