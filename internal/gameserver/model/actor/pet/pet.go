// Package pet models a tamed creature's growth, feeding, and persisted
// vitals: the data and formulas needed to grow, feed, and save/restore one,
// independent of how it is spawned, moved, or fought with in the world.
package pet

// State is a pet's persisted vitals and growth progress: one row of its
// saved data, keyed elsewhere by the collar item that owns it.
type State struct {
	Name  string
	Level int
	Exp   int64
	SP    int
	CurHP float64
	CurMP float64
	// Fed is the current meal gauge, in the same units as a level row's
	// max meal value.
	Fed int
}

// sinEaterNpcID is the one pet npc id whose level always tracks its owner's
// current level rather than growing through its own exp table, and whose
// exp gain uses a separately configured rate.
const sinEaterNpcID = 12564

// mountableNpcIDs are the pet npc ids that can be ridden by their owner.
var mountableNpcIDs = map[int]bool{
	12526: true,
	12527: true,
	12528: true,
	12621: true,
}

// IsMountable reports whether a pet of npcID can be ridden by its owner.
func IsMountable(npcID int) bool {
	return mountableNpcIDs[npcID]
}

// TracksOwnerLevel reports whether a pet of npcID always starts at (and
// stays pinned to) its owner's level instead of leveling up through its own
// exp table.
func TracksOwnerLevel(npcID int) bool {
	return npcID == sinEaterNpcID
}

// InitialLevel returns the level a freshly created pet of npcID starts at,
// when no saved row exists for it yet: templateLevel for an ordinary pet,
// or the owner's current level for one that tracks its owner (see
// TracksOwnerLevel).
func InitialLevel(npcID, templateLevel, ownerLevel int) int {
	if TracksOwnerLevel(npcID) {
		return ownerLevel
	}
	return templateLevel
}

// ScaledExpGain applies the configured pet experience-rate multiplier to a
// raw exp reward. A pet that TracksOwnerLevel uses its own separately
// configured rate instead of the general pet rate; sp is never rate-scaled.
func ScaledExpGain(npcID int, rawExp int64, petRate, trackingPetRate float64) int64 {
	rate := petRate
	if TracksOwnerLevel(npcID) {
		rate = trackingPetRate
	}
	return roundInt64(float64(rawExp) * rate)
}
