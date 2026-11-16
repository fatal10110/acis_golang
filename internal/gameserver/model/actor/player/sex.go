package player

import "fmt"

// Sex is a character's sex, encoded on the wire and in the characters table
// as 0 (male), 1 (female) or 2 (etc). A real client only creates 0 or 1,
// but creation accepts 2 as well, and an admin can set it.
type Sex byte

const (
	SexMale Sex = iota
	SexFemale
	SexEtc
)

// String returns the client-facing sex name.
func (s Sex) String() string {
	switch s {
	case SexMale:
		return "male"
	case SexFemale:
		return "female"
	case SexEtc:
		return "etc"
	default:
		return fmt.Sprintf("sex(%d)", byte(s))
	}
}

// Sex returns c's sex.
func (c *Character) Sex() Sex { return Sex(c.sex.Load()) }

// SetSex changes c's sex.
func (c *Character) SetSex(s Sex) { c.sex.Store(uint32(s)) }
