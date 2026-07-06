package player

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
