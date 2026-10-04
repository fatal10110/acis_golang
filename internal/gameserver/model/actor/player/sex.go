package player

import "fmt"

// Sex is a character's sex, encoded on the wire and in the characters table
// as 0 (male) or 1 (female). 2 (etc) is no sex a client creates, but an
// admin can set it.
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

// ParseSex validates a wire-supplied sex value. Only 0 and 1 are ever sent
// by a real client; anything else is rejected rather than stored, since
// nothing downstream (hairstyle bounds, appearance) has a defined meaning
// for it.
func ParseSex(v int32) (Sex, error) {
	switch v {
	case 0:
		return SexMale, nil
	case 1:
		return SexFemale, nil
	default:
		return 0, fmt.Errorf("player: invalid sex value %d", v)
	}
}

// Sex returns c's sex.
func (c *Character) Sex() Sex { return Sex(c.sex.Load()) }

// SetSex changes c's sex.
func (c *Character) SetSex(s Sex) { c.sex.Store(uint32(s)) }
