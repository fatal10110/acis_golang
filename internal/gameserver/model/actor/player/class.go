package player

import "slices"

// classParent maps a profession id to the id of the profession it upgrades
// from, or -1 for one of the 9 base professions. Ids cover exactly the
// professions the class template data defines: 0-57 for the base, first and
// second tier professions across the 9 lines, and 88-118 for the third
// tier. The 30 ids in between are reserved by the data format and never
// assigned to a profession. Race, class names and the rest of the
// profession enumeration arrive with the character-creation work.
//
// Every parent id is numerically smaller than all of its children's ids;
// NewTemplateTable's single ascending skill-merge pass relies on that.
var classParent = map[int]int{
	0: -1, 1: 0, 2: 1, 3: 1, 4: 0, 5: 4, 6: 4, 7: 0, 8: 7, 9: 7,
	10: -1, 11: 10, 12: 11, 13: 11, 14: 11, 15: 10, 16: 15, 17: 15,
	18: -1, 19: 18, 20: 19, 21: 19, 22: 18, 23: 22, 24: 22,
	25: -1, 26: 25, 27: 26, 28: 26, 29: 25, 30: 29,
	31: -1, 32: 31, 33: 32, 34: 32, 35: 31, 36: 35, 37: 35,
	38: -1, 39: 38, 40: 39, 41: 39, 42: 38, 43: 42,
	44: -1, 45: 44, 46: 45, 47: 44, 48: 47,
	49: -1, 50: 49, 51: 50, 52: 50,
	53: -1, 54: 53, 55: 54, 56: 53, 57: 56,

	88: 2, 89: 3, 90: 5, 91: 6, 92: 9, 93: 8, 94: 12, 95: 13, 96: 14, 97: 16, 98: 17,
	99: 20, 100: 21, 101: 23, 102: 24, 103: 27, 104: 28, 105: 30,
	106: 33, 107: 34, 108: 36, 109: 37, 110: 40, 111: 41, 112: 43,
	113: 46, 114: 48, 115: 51, 116: 52,
	117: 55, 118: 57,
}

// ClassParent returns the id of the profession that id upgrades from (-1
// for a base profession), and whether id is a known profession at all.
func ClassParent(id int) (int, bool) {
	p, ok := classParent[id]
	return p, ok
}

// ClassLevel returns id's profession tier — 0 for a base profession, 1 or 2
// for a first or second occupation change, 3 for a third-class/awakened
// profession — counted by walking classParent to its root, and whether id
// is a known profession at all.
func ClassLevel(id int) (int, bool) {
	parent, ok := classParent[id]
	if !ok {
		return 0, false
	}
	level := 0
	for parent != -1 {
		level++
		next, ok := classParent[parent]
		if !ok {
			return 0, false
		}
		parent = next
	}
	return level, true
}

// LowLevelNewbie reports whether c is a newbie of level 6 to 25: one who
// has made at most the first occupation change.
func (c *Character) LowLevelNewbie() bool {
	tier, _ := ClassLevel(c.ClassID())
	level := c.Level()
	return tier <= 1 && level >= 6 && level <= 25
}

// ClassType is a profession's teaching line, which decides the village
// masters that handle it.
type ClassType int

const (
	ClassTypeFighter ClassType = iota
	ClassTypeMystic
	ClassTypePriest
)

// classInfo is a profession's client-facing name and teaching line.
type classInfo struct {
	name string
	typ  ClassType
}

// classInfos holds every profession of classParent, by id.
var classInfos = map[int]classInfo{
	0: {"Human Fighter", ClassTypeFighter}, 1: {"Warrior", ClassTypeFighter}, 2: {"Gladiator", ClassTypeFighter},
	3: {"Warlord", ClassTypeFighter}, 4: {"Human Knight", ClassTypeFighter}, 5: {"Paladin", ClassTypeFighter},
	6: {"Dark Avenger", ClassTypeFighter}, 7: {"Rogue", ClassTypeFighter}, 8: {"Treasure Hunter", ClassTypeFighter},
	9:  {"Hawkeye", ClassTypeFighter},
	10: {"Human Mystic", ClassTypeMystic}, 11: {"Human Wizard", ClassTypeMystic}, 12: {"Sorcerer", ClassTypeMystic},
	13: {"Necromancer", ClassTypeMystic}, 14: {"Warlock", ClassTypeMystic}, 15: {"Cleric", ClassTypePriest},
	16: {"Bishop", ClassTypePriest}, 17: {"Prophet", ClassTypePriest},
	18: {"Elven Fighter", ClassTypeFighter}, 19: {"Elven Knight", ClassTypeFighter}, 20: {"Temple Knight", ClassTypeFighter},
	21: {"Sword Singer", ClassTypeFighter}, 22: {"Elven Scout", ClassTypeFighter}, 23: {"Plains Walker", ClassTypeFighter},
	24: {"Silver Ranger", ClassTypeFighter},
	25: {"Elven Mystic", ClassTypeMystic}, 26: {"Elven Wizard", ClassTypeMystic}, 27: {"Spellsinger", ClassTypeMystic},
	28: {"Elemental Summoner", ClassTypeMystic}, 29: {"Elven Oracle", ClassTypePriest}, 30: {"Elven Elder", ClassTypePriest},
	31: {"Dark Fighter", ClassTypeFighter}, 32: {"Palus Knight", ClassTypeFighter}, 33: {"Shillien Knight", ClassTypeFighter},
	34: {"Bladedancer", ClassTypeFighter}, 35: {"Assassin", ClassTypeFighter}, 36: {"Abyss Walker", ClassTypeFighter},
	37: {"Phantom Ranger", ClassTypeFighter},
	38: {"Dark Mystic", ClassTypeMystic}, 39: {"Dark Wizard", ClassTypeMystic}, 40: {"Spellhowler", ClassTypeMystic},
	41: {"Phantom Summoner", ClassTypeMystic}, 42: {"Shillien Oracle", ClassTypePriest}, 43: {"Shillien Elder", ClassTypePriest},
	44: {"Orc Fighter", ClassTypeFighter}, 45: {"Orc Raider", ClassTypeFighter}, 46: {"Destroyer", ClassTypeFighter},
	47: {"Monk", ClassTypeFighter}, 48: {"Tyrant", ClassTypeFighter},
	49: {"Orc Mystic", ClassTypeMystic}, 50: {"Orc Shaman", ClassTypeMystic}, 51: {"Overlord", ClassTypeMystic},
	52: {"Warcryer", ClassTypeMystic},
	53: {"Dwarven Fighter", ClassTypeFighter}, 54: {"Scavenger", ClassTypeFighter}, 55: {"Bounty Hunter", ClassTypeFighter},
	56: {"Artisan", ClassTypeFighter}, 57: {"Warsmith", ClassTypeFighter},

	88: {"Duelist", ClassTypeFighter}, 89: {"Dreadnought", ClassTypeFighter}, 90: {"Phoenix Knight", ClassTypeFighter},
	91: {"Hell Knight", ClassTypeFighter}, 92: {"Sagittarius", ClassTypeFighter}, 93: {"Adventurer", ClassTypeFighter},
	94: {"Archmage", ClassTypeMystic}, 95: {"Soultaker", ClassTypeMystic}, 96: {"Arcana Lord", ClassTypeMystic},
	97: {"Cardinal", ClassTypePriest}, 98: {"Hierophant", ClassTypePriest},
	99: {"Eva's Templar", ClassTypeFighter}, 100: {"Sword Muse", ClassTypeFighter}, 101: {"Wind Rider", ClassTypeFighter},
	102: {"Moonlight Sentinel", ClassTypeFighter}, 103: {"Mystic Muse", ClassTypeMystic}, 104: {"Elemental Master", ClassTypeMystic},
	105: {"Eva's Saint", ClassTypePriest},
	106: {"Shillien Templar", ClassTypeFighter}, 107: {"Spectral Dancer", ClassTypeFighter}, 108: {"Ghost Hunter", ClassTypeFighter},
	109: {"Ghost Sentinel", ClassTypeFighter}, 110: {"Storm Screamer", ClassTypeMystic}, 111: {"Spectral Master", ClassTypeMystic},
	112: {"Shillien Saint", ClassTypePriest},
	113: {"Titan", ClassTypeFighter}, 114: {"Grand Khavatari", ClassTypeFighter}, 115: {"Dominator", ClassTypeMystic},
	116: {"Doom Cryer", ClassTypeMystic},
	117: {"Fortune Seeker", ClassTypeFighter}, 118: {"Maestro", ClassTypeFighter},
}

// ClassName returns id's profession name, or "Invalid class" for an id
// that names none.
func ClassName(id int) string {
	if info, ok := classInfos[id]; ok {
		return info.name
	}
	return "Invalid class"
}

// ClassTypeOf returns id's teaching line, and whether id is a known
// profession.
func ClassTypeOf(id int) (ClassType, bool) {
	info, ok := classInfos[id]
	return info.typ, ok
}

// ClassEqualsOrChildOf reports whether id is ancestor or one of its
// upgrades.
func ClassEqualsOrChildOf(id, ancestor int) bool {
	for {
		if id == ancestor {
			return true
		}
		parent, ok := classParent[id]
		if !ok || parent < 0 {
			return false
		}
		id = parent
	}
}

// subclassExclusive are the groups of second professions a member of the
// group may not take another member of as a subclass.
var subclassExclusive = [][]int{
	{6, 5, 20, 33}, // Dark Avenger, Paladin, Temple Knight, Shillien Knight
	{8, 36, 23},    // Treasure Hunter, Abyss Walker, Plains Walker
	{9, 24, 37},    // Hawkeye, Silver Ranger, Phantom Ranger
	{14, 28, 41},   // Warlock, Elemental Summoner, Phantom Summoner
	{12, 27, 40},   // Sorcerer, Spellsinger, Spellhowler
}

// AvailableSubclasses lists, by ascending id, the second professions a
// character of base class baseClassID may take as a subclass. Only a second
// or third profession may take one, a third one as its second profession
// does. Overlord, Warsmith and the base's own profession are never offered,
// elves and dark elves may not take each other's professions, and a member
// of one of the exclusive groups none of its group.
func AvailableSubclasses(baseClassID int) []int {
	base := baseClassID
	tier, ok := ClassLevel(base)
	if !ok || tier < 2 {
		return nil
	}
	if tier == 3 {
		base = classParent[base]
	}
	baseRace, _ := ClassRace(base)
	var excluded []int
	for _, group := range subclassExclusive {
		if slices.Contains(group, base) {
			excluded = group
			break
		}
	}
	var out []int
	for id := 0; id <= maxClassID; id++ {
		if tier, ok := ClassLevel(id); !ok || tier != 2 {
			continue
		}
		if id == 51 || id == 57 || id == base || slices.Contains(excluded, id) {
			continue
		}
		race, _ := ClassRace(id)
		if (baseRace == RaceElf && race == RaceDarkElf) || (baseRace == RaceDarkElf && race == RaceElf) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// maxClassID is the highest profession id.
const maxClassID = 118
