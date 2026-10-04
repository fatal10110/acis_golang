package npc

// raceEnumNames is ordered by Race value: each race's constant name in the
// reference NpcRace enum, the spelling admin pages print.
var raceEnumNames = [...]string{
	"DUMMY", "UNDEAD", "MAGIC_CREATURE", "BEAST", "ANIMAL", "PLANT", "HUMANOID",
	"SPIRIT", "ANGEL", "DEMON", "DRAGON", "GIANT", "BUG", "FAIRIE", "HUMAN",
	"ELVE", "DARKELVE", "ORC", "DWARVE", "OTHER", "NON_LIVING_BEING", "SIEGE_WEAPON",
	"DEFENDING_ARMY", "MERCENARIE", "UNKNOWN_CREATURE",
}

// EnumName returns r's constant name in the reference NpcRace enum, empty
// for a value past the last race.
func (r Race) EnumName() string {
	if int(r) < len(raceEnumNames) {
		return raceEnumNames[r]
	}
	return ""
}
