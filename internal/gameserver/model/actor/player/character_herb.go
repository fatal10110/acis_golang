package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// ConsumeHerb applies the herb itemID carries to this character and reports
// whether anything was there to apply it. A character with no sink, or whose
// session has detached, has nothing, so the caller can still deliver the herb
// some other way instead of dropping it.
func (c *Character) ConsumeHerb(itemID int32) bool {
	if c.sink == nil || c.SessionDetached() {
		return false
	}
	c.emit(event.HerbConsumed{ItemID: itemID})
	return true
}
