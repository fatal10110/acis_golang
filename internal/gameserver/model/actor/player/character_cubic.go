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
// party member (rather than c itself) granted it, or resets an
// already-active cubic's own disappear timer instead. added reports
// whether a new cubic was actually admitted, so the caller knows whether
// to refresh the character-info the client sees.
func (c *Character) AddOrRefreshCubic(id cubic.ID, givenByOther bool) (added bool) {
	refreshed, _, _ := c.cubics.AddOrRefresh(id, givenByOther, c.cubicMaxSlots())
	return !refreshed
}

// CubicIDs returns the ids of c's currently active cubics, in grant order,
// for packet serialization.
func (c *Character) CubicIDs() []int {
	return c.cubics.IDs()
}
