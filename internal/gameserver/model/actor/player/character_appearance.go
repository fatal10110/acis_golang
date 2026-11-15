package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// Appearance returns c's face, hair style and hair color.
func (c *Character) Appearance() (face, hairStyle, hairColor int) {
	c.appearanceMu.RLock()
	defer c.appearanceMu.RUnlock()
	return c.Face, c.HairStyle, c.HairColor
}

// ChangeAppearance gives c the face, hair color and hair style passed,
// leaving each one passed negative as it is, and then shows c anew to
// itself and every player around it (see BroadcastUserInfo).
func (c *Character) ChangeAppearance(face, hairColor, hairStyle int) {
	c.appearanceMu.Lock()
	if face >= 0 {
		c.Face = face
	}
	if hairColor >= 0 {
		c.HairColor = hairColor
	}
	if hairStyle >= 0 {
		c.HairStyle = hairStyle
	}
	c.appearanceMu.Unlock()
	c.emit(event.UserInfoBroadcast{})
}
