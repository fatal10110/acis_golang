package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// RightHandItemID returns the item id of what c holds in its right hand, 0
// when the hand is empty.
func (c *Character) RightHandItemID() int32 {
	if c.inventory == nil {
		return 0
	}
	if inst := c.inventory.ItemAt(itemcontainer.RHand); inst != nil {
		return inst.TemplateID
	}
	return 0
}

// RefreshScriptHennaList recomputes what c's dyes add to its stats and
// shows c its dye list, as a script asks after a class change.
func (c *Character) RefreshScriptHennaList() {
	c.RefreshHennaStats()
	c.emit(event.HennaListShown{})
}

// MakeScriptNoble makes c a noblesse, as a script rewards it. A character
// leaving the world is left as it is, as no script gives it anything.
func (c *Character) MakeScriptNoble() {
	c.whileAttached(func() { c.emit(event.NobleGranted{}) })
}

// Sponsor returns the object id of c's academy sponsor in its clan, 0 for
// none.
func (c *Character) Sponsor() int32 {
	sponsor, _ := c.mentors()
	return sponsor
}

// Apprentice returns the object id of c's academy apprentice in its clan, 0
// for none.
func (c *Character) Apprentice() int32 {
	_, apprentice := c.mentors()
	return apprentice
}

func (c *Character) mentors() (sponsor, apprentice int32) {
	clanID := c.ClanID()
	if clanID == 0 || c.social == nil {
		return 0, 0
	}
	return c.social.Mentors(clanID, c.ObjectID())
}
