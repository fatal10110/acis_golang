package npc

import "fmt"

// Race classifies an NPC template for the resistance/attack-bonus rules
// that key off race elsewhere in the data. It is derived at load time from
// the template's <skills> block rather than read from a "race" attribute
// directly (see RaceBySecondarySkillID and RaceSkillID).
type Race uint8

const (
	RaceDummy Race = iota
	RaceUndead
	RaceMagicCreature
	RaceBeast
	RaceAnimal
	RacePlant
	RaceHumanoid
	RaceSpirit
	RaceAngel
	RaceDemon
	RaceDragon
	RaceGiant
	RaceBug
	RaceFairy
	RaceHuman
	RaceElf
	RaceDarkElf
	RaceOrc
	RaceDwarf
	RaceOther
	RaceNonLivingBeing
	RaceSiegeWeapon
	RaceDefendingArmy
	RaceMercenary
	RaceUnknownCreature
)

// raceNames is ordered by Race value; its index doubles as each race's
// String() lookup and RaceByOrdinal's bounds.
var raceNames = [...]string{
	"Dummy", "Undead", "MagicCreature", "Beast", "Animal", "Plant", "Humanoid",
	"Spirit", "Angel", "Demon", "Dragon", "Giant", "Bug", "Fairy", "Human",
	"Elf", "DarkElf", "Orc", "Dwarf", "Other", "NonLivingBeing", "SiegeWeapon",
	"DefendingArmy", "Mercenary", "UnknownCreature",
}

// String returns r's name.
func (r Race) String() string {
	if int(r) < len(raceNames) {
		return raceNames[r]
	}
	return fmt.Sprintf("Race(%d)", uint8(r))
}

// secondarySkillRace maps the id of a race-marker skill to the Race it
// identifies. A handful of races are only ever detected this way; the rest
// have no secondary skill and are only reachable through RaceByOrdinal.
var secondarySkillRace = map[int]Race{
	4290: RaceUndead,
	4291: RaceMagicCreature,
	4292: RaceBeast,
	4293: RaceAnimal,
	4294: RacePlant,
	4299: RaceDragon,
	4300: RaceGiant,
	4301: RaceBug,
	4302: RaceFairy,
}

// RaceBySecondarySkillID returns the Race a template's race-marker skill id
// identifies, or RaceDummy if skillID names no race.
func RaceBySecondarySkillID(skillID int) Race {
	return secondarySkillRace[skillID]
}

// RaceSkillID is the skill id whose level directly encodes a template's
// Race (via RaceByOrdinal) when no secondary race-marker skill applies.
const RaceSkillID = 4416

// RaceByOrdinal returns the Race at position n, and whether n is in range.
func RaceByOrdinal(n int) (Race, bool) {
	if n < 0 || n >= len(raceNames) {
		return 0, false
	}
	return Race(n), true
}
