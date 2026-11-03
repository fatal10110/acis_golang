package npc

// raceEnumNames is ordered by Race value: each race's upper-case constant
// name, the spelling admin pages print.
var raceEnumNames = [...]string{
	"DUMMY", "UNDEAD", "MAGIC_CREATURE", "BEAST", "ANIMAL", "PLANT", "HUMANOID",
	"SPIRIT", "ANGEL", "DEMON", "DRAGON", "GIANT", "BUG", "FAIRIE", "HUMAN",
	"ELVE", "DARKELVE", "ORC", "DWARVE", "OTHER", "NON_LIVING_BEING", "SIEGE_WEAPON",
	"DEFENDING_ARMY", "MERCENARIE", "UNKNOWN_CREATURE",
}

// EnumName returns r's upper-case constant name as admin pages print it,
// empty for a value past the last race.
func (r Race) EnumName() string {
	if int(r) < len(raceEnumNames) {
		return raceEnumNames[r]
	}
	return ""
}
