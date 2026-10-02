package sevensigns

import "fmt"

// Cabal is the side of the competition a player signed up for. The numeric
// values are the ones the client reads.
type Cabal int

const (
	// NoCabal is a player who has not signed up, and an unowned seal.
	NoCabal Cabal = iota
	Dusk
	Dawn
)

// String returns the persisted enum name of c.
func (c Cabal) String() string {
	switch c {
	case NoCabal:
		return "NORMAL"
	case Dusk:
		return "DUSK"
	case Dawn:
		return "DAWN"
	default:
		return fmt.Sprintf("Cabal(%d)", int(c))
	}
}

// ParseCabal parses a persisted enum name.
func ParseCabal(name string) (Cabal, error) {
	for c := NoCabal; c <= Dawn; c++ {
		if c.String() == name {
			return c, nil
		}
	}
	return 0, fmt.Errorf("unknown seven signs cabal %q", name)
}

// Seal is one of the three seals the cabals compete for, or none. The
// numeric values are the ones the client reads.
type Seal int

const (
	NoSeal Seal = iota
	Avarice
	Gnosis
	Strife
)

// Seals lists the three contested seals in their fixed order.
var Seals = [...]Seal{Avarice, Gnosis, Strife}

// String returns the persisted enum name of s.
func (s Seal) String() string {
	switch s {
	case NoSeal:
		return "NONE"
	case Avarice:
		return "AVARICE"
	case Gnosis:
		return "GNOSIS"
	case Strife:
		return "STRIFE"
	default:
		return fmt.Sprintf("Seal(%d)", int(s))
	}
}

// ParseSeal parses a persisted enum name.
func ParseSeal(name string) (Seal, error) {
	for s := NoSeal; s <= Strife; s++ {
		if s.String() == name {
			return s, nil
		}
	}
	return 0, fmt.Errorf("unknown seven signs seal %q", name)
}

// sealIndex is s's slot in the per-seal arrays, Avarice first; ok is false
// for NoSeal and unknown values.
func sealIndex(s Seal) (int, bool) {
	if s < Avarice || s > Strife {
		return 0, false
	}
	return int(s - Avarice), true
}
