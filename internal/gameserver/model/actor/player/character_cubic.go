package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"

// cubicMasterySkillID is Cubic Mastery, the skill that raises how many
// cubics a player can hold active at once beyond the default of one.
const cubicMasterySkillID = 143

// cubicMaxSlots is the mastery level driving c's cubic cap: a cubic list
// is full once it holds more than this many cubics.
func (c *Character) cubicMaxSlots() int {
	return c.SkillLevel(cubicMasterySkillID)
}

// CubicListFull reports whether c already holds as many active cubics as
// Cubic Mastery allows, matching the reference's CubicList.isFull().
func (c *Character) CubicListFull() bool {
	return c.cubics.Len() > c.cubicMaxSlots()
}

// AddOrRefreshCubic admits id to c's active cubics, recording whether a
// party member (rather than c itself) granted it, or marks an already-active
// cubic for a disappear-timer reset instead. touched reports whether id is
// now active in c's list either way, so the caller can (re)sync its live
// cubic runtime's disappear timer; added reports whether it was newly
// admitted rather than refreshed, so the caller knows whether to refresh the
// character-info the client sees.
func (c *Character) AddOrRefreshCubic(id cubic.ID, givenByOther bool) (touched, added bool) {
	refreshed, _, _ := c.cubics.AddOrRefresh(id, givenByOther, c.cubicMaxSlots())
	return true, !refreshed
}

// RemoveCubic deactivates the cubic of id, matching Cubic.stop() removing
// itself from the owner's CubicList once its lifetime elapses or it is
// otherwise stopped.
func (c *Character) RemoveCubic(id cubic.ID) {
	c.cubics.Remove(id)
}

// CubicIDs returns the ids of c's currently active cubics, in grant order,
// for packet serialization.
func (c *Character) CubicIDs() []int {
	return c.cubics.IDs()
}
