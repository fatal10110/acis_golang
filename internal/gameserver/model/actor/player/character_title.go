package player

// Colors a character's name and title are drawn in until something sets
// others.
const (
	DefaultNameColor  int32 = 0xFFFFFF
	DefaultTitleColor int32 = 0xFFFF77
)

// nameColors is a character's name and title colors.
type nameColors struct {
	name, title int32
}

// NameColor is the color the character's name is drawn in.
func (c *Character) NameColor() int32 {
	if p := c.colors.Load(); p != nil {
		return p.name
	}
	return DefaultNameColor
}

// TitleColor is the color the character's title is drawn in.
func (c *Character) TitleColor() int32 {
	if p := c.colors.Load(); p != nil {
		return p.title
	}
	return DefaultTitleColor
}

// SetColors replaces the colors the character's name and title are drawn
// in.
func (c *Character) SetColors(name, title int32) {
	c.colors.Store(&nameColors{name: name, title: title})
}
