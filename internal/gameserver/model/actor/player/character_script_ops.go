package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// IsNewbie reports whether c counts as a newbie: at most one class change
// made and level 25 or lower, and, with checkLowLevel, level 6 or higher
// (Player.isNewbie).
func (c *Character) IsNewbie(checkLowLevel bool) bool {
	tier, _ := ClassLevel(c.ClassID())
	level := c.Level()
	if tier > 1 || level > 25 {
		return false
	}
	return !checkLowLevel || level >= 6
}

// ClanLevel returns the level of c's clan; ok is false when c is in no
// clan.
func (c *Character) ClanLevel() (level int, ok bool) {
	clanID := c.ClanID()
	if clanID == 0 {
		return 0, false
	}
	if c.social == nil {
		return 0, true
	}
	return c.social.ClanLevel(clanID), true
}

// AddScriptClanReputation adds points to the reputation of c's clan, as a
// script rewards it: the clan's online members see the new score as on any
// reputation change, and a clan below level 5 gains nothing. A character
// leaving the world gets nothing, as no script gives it anything.
func (c *Character) AddScriptClanReputation(points int) {
	if c.Detaching() {
		return
	}
	c.emit(event.ClanReputationAdded{Points: points})
}

// BroadcastClanInfo shows every online member of c's clan the clan's
// header.
func (c *Character) BroadcastClanInfo() {
	c.emit(event.ClanInfoBroadcast{})
}

// BroadcastSocialAction plays the social animation id on c for c and every
// player around it.
func (c *Character) BroadcastSocialAction(id int32) {
	c.emit(event.SocialAction{ID: id})
}

// NotifySystemMessageNumbers shows c the system message id with numbers
// as its parameters, in order.
func (c *Character) NotifySystemMessageNumbers(id int, numbers ...int32) {
	c.emit(event.SystemMessageShown{ID: id, Numbers: numbers})
}

// PlayScene plays the client's built-in scene for c alone.
func (c *Character) PlayScene() {
	c.emit(event.ScenePlayed{})
}
