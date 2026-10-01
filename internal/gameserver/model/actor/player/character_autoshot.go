package player

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// AutoSoulShotStatus describes the outcome of an auto-shot toggle request.
type AutoSoulShotStatus uint8

const (
	// AutoSoulShotToggled means the auto-shot state was updated.
	AutoSoulShotToggled AutoSoulShotStatus = iota
	// AutoSoulShotNoop means the request should be ignored.
	AutoSoulShotNoop
	// AutoSoulShotNeedsSummon means a summon shot was requested without an active summon.
	AutoSoulShotNeedsSummon
)

// SetAutoSoulShot records whether itemID is active for automatic shot use.
func (c *Character) SetAutoSoulShot(itemID int32, enabled bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if enabled {
		if c.autoSoulShots == nil {
			c.autoSoulShots = make(map[int32]bool)
		}
		c.autoSoulShots[itemID] = true
		return
	}
	delete(c.autoSoulShots, itemID)
}

// RemoveAutoSoulShot turns automatic use of itemID off and reports whether
// it was on.
func (c *Character) RemoveAutoSoulShot(itemID int32) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if !c.autoSoulShots[itemID] {
		return false
	}
	delete(c.autoSoulShots, itemID)
	return true
}

// AutoSoulShotEnabled reports whether itemID is active for automatic shot use.
func (c *Character) AutoSoulShotEnabled(itemID int32) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.autoSoulShots[itemID]
}

// AutoSoulShotIDs returns the item ids active for automatic shot use, in
// ascending order.
func (c *Character) AutoSoulShotIDs() []int32 {
	c.stateMu.RLock()
	ids := make([]int32, 0, len(c.autoSoulShots))
	for id := range c.autoSoulShots {
		ids = append(ids, id)
	}
	c.stateMu.RUnlock()
	slices.Sort(ids)
	return ids
}

// ToggleAutoSoulShot applies auto-shot item rules and records the new state.
func (c *Character) ToggleAutoSoulShot(itemID int32, enabled, hasItem, hasSummon bool) AutoSoulShotStatus {
	if c == nil || !hasItem {
		return AutoSoulShotNoop
	}
	if enabled {
		if item.IsFishingShotID(itemID) {
			return AutoSoulShotNoop
		}
		if item.IsSummonShotID(itemID) && !hasSummon {
			return AutoSoulShotNeedsSummon
		}
	}
	c.SetAutoSoulShot(itemID, enabled)
	return AutoSoulShotToggled
}
