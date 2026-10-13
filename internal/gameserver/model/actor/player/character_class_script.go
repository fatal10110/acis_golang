package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// RequestClassChange moves c to classID as a script asks: the whole occupation
// change, done by c's controller before the call returns. Nothing happens while
// another class change of c is in progress.
func (c *Character) RequestClassChange(classID int) {
	c.emit(event.ClassChangeRequested{ClassID: classID})
}

// RequestBaseClass makes classID c's base class as a script asks, with
// that class's template as the base body.
func (c *Character) RequestBaseClass(classID int) {
	c.emit(event.BaseClassChangeRequested{ClassID: classID})
}

// BroadcastUserInfo shows c anew: its own full view to c, its public view to
// every player around it.
func (c *Character) BroadcastUserInfo() {
	c.emit(event.UserInfoBroadcast{})
}

// BroadcastSkillAnimation shows c and every player around it the cast
// animation of skillID at level from casterID, standing at casterAt, to
// targetID, standing at targetAt (a MagicSkillUse). It shows nothing more:
// no skill is used.
func (c *Character) BroadcastSkillAnimation(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int) {
	c.emit(event.MagicSkillUse{
		CasterID: casterID, CasterAt: casterAt,
		TargetID: targetID, TargetAt: targetAt,
		SkillID: skillID, Level: level, HitTime: hitTime, ReuseDelay: reuseDelay,
	})
}

// ActiveWeaponItemID returns the item id of the weapon c attacks with: the one
// in its right hand, or its class's fists when that hand holds no weapon; 0
// when its class has no fists.
func (c *Character) ActiveWeaponItemID() int32 {
	if tmpl := c.activeWeapon().tmpl; tmpl != nil {
		return tmpl.ID
	}
	return 0
}
