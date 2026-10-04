package player

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// AutoSoulShotStatus describes the outcome of an auto-shot toggle request.
type AutoSoulShotStatus uint8

const (
	// AutoSoulShotToggled means the auto-shot state was updated. An enabled
	// weapon shot matches the active weapon's grade, so the weapon is
	// charged from the auto-use shots.
	AutoSoulShotToggled AutoSoulShotStatus = iota
	// AutoSoulShotNoop means the request should be ignored.
	AutoSoulShotNoop
	// AutoSoulShotNeedsSummon means a summon shot was requested without an active summon.
	AutoSoulShotNeedsSummon
	// AutoSoulShotOlympiadBlocked means a blessed spiritshot (weapon or
	// servitor) was requested during an Olympiad match.
	AutoSoulShotOlympiadBlocked
	// AutoSoulShotNotEnoughForPet means the held servitor shot stack is
	// smaller than what the summon spends per charge.
	AutoSoulShotNotEnoughForPet
	// AutoSoulShotGradeMismatch means a weapon shot was enabled although no
	// weapon is held or its grade differs from the shot's: auto use is on,
	// but nothing is charged.
	AutoSoulShotGradeMismatch
)

// AutoShotSummon is the active pet or servitor a servitor shot charges.
type AutoShotSummon interface {
	SSCount() int
	SPSCount() int
}

// AutoSoulShotRequest is one auto-shot toggle request against the
// player's current state.
type AutoSoulShotRequest struct {
	ItemID  int32
	Enabled bool
	// Held reports whether an ItemID stack is held; ItemCount is its count.
	Held      bool
	ItemCount int
	// Summon is the active pet or servitor, or nil when there is none.
	Summon AutoShotSummon
}

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

// ToggleAutoSoulShot applies the auto-shot item rules to req and records
// the new state. Only AutoSoulShotToggled and AutoSoulShotGradeMismatch
// change it; every other status leaves auto use as it was.
func (c *Character) ToggleAutoSoulShot(req AutoSoulShotRequest) AutoSoulShotStatus {
	if c == nil || !req.Held {
		return AutoSoulShotNoop
	}
	status := AutoSoulShotToggled
	if req.Enabled {
		status = autoShotEnableGate(req, c.OlympiadMode())
		switch status {
		case AutoSoulShotToggled:
			if !item.IsSummonShotID(req.ItemID) && !c.autoShotGradeMatches(req.ItemID) {
				status = AutoSoulShotGradeMismatch
			}
		default:
			return status
		}
	}
	c.SetAutoSoulShot(req.ItemID, req.Enabled)
	return status
}

// autoShotEnableGate runs the checks that refuse to turn auto use of
// req.ItemID on, in their order, for a player who is in an Olympiad match
// when olympiad is set. Servitor shots need a summon first, then the
// blessed one is refused in Olympiad, then the stack must cover one charge
// of the summon; weapon blessed spiritshots are refused in Olympiad.
func autoShotEnableGate(req AutoSoulShotRequest, olympiad bool) AutoSoulShotStatus {
	switch {
	case item.IsFishingShotID(req.ItemID):
		return AutoSoulShotNoop
	case item.IsSummonShotID(req.ItemID):
		if req.Summon == nil {
			return AutoSoulShotNeedsSummon
		}
		if req.ItemID == item.BlessedBeastSpiritshotID && olympiad {
			return AutoSoulShotOlympiadBlocked
		}
		perCharge := req.Summon.SPSCount()
		if req.ItemID == item.BeastSoulshotID {
			perCharge = req.Summon.SSCount()
		}
		if perCharge > req.ItemCount {
			return AutoSoulShotNotEnoughForPet
		}
	case item.IsBlessedSpiritshotID(req.ItemID) && olympiad:
		return AutoSoulShotOlympiadBlocked
	}
	return AutoSoulShotToggled
}

// autoShotGradeMatches reports whether a weapon is held whose crystal grade
// is itemID's.
func (c *Character) autoShotGradeMatches(itemID int32) bool {
	w := c.activeWeapon()
	if w.inst == nil || w.tmpl == nil || c.inventory == nil {
		return false
	}
	shot, ok := c.inventory.Templates().Get(itemID)
	return ok && shot != nil && shot.Crystal == w.tmpl.Crystal
}
