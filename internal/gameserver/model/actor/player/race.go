package player

import "fmt"

// Race identifies a player character's playable race. Every profession id
// belongs to exactly one race, shared by the whole profession line (a class
// and all of its upgrades).
type Race int

const (
	RaceHuman Race = iota
	RaceElf
	RaceDarkElf
	RaceOrc
	RaceDwarf
)

// String returns the client-facing race name.
func (r Race) String() string {
	switch r {
	case RaceHuman:
		return "Human"
	case RaceElf:
		return "Elf"
	case RaceDarkElf:
		return "DarkElf"
	case RaceOrc:
		return "Orc"
	case RaceDwarf:
		return "Dwarf"
	default:
		return fmt.Sprintf("Race(%d)", int(r))
	}
}

// baseClassRace gives the race of each of the 9 root professions (the ids
// for which ClassParent reports no parent). Every other profession in a
// line shares its root's race; ClassRace walks ClassParent to find it
// rather than repeating this table for all ~110 ids.
var baseClassRace = map[int]Race{
	0:  RaceHuman,   // Human Fighter
	10: RaceHuman,   // Human Mystic
	18: RaceElf,     // Elven Fighter
	25: RaceElf,     // Elven Mystic
	31: RaceDarkElf, // Dark Fighter
	38: RaceDarkElf, // Dark Mystic
	44: RaceOrc,     // Orc Fighter
	49: RaceOrc,     // Orc Mystic
	53: RaceDwarf,   // Dwarven Fighter
}

// ClassRace returns the race of the profession line id belongs to, and
// whether id is a known profession at all.
func ClassRace(id int) (Race, bool) {
	for {
		parent, ok := ClassParent(id)
		if !ok {
			return 0, false
		}
		if parent < 0 {
			race, ok := baseClassRace[id]
			return race, ok
		}
		id = parent
	}
}
