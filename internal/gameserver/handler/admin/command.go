package admin

import "strings"

// Word returns command's command word: everything before its first space.
func Word(command string) string {
	if i := strings.IndexByte(command, ' '); i >= 0 {
		return command[:i]
	}
	return command
}

// Args returns the arguments after command's word: the command split on
// runs of spaces, tabs, line breaks and form feeds, the word dropped.
func Args(command string) []string {
	tokens := strings.FieldsFunc(command, isDelimiter)
	if len(tokens) == 0 {
		return nil
	}
	return tokens[1:]
}

func isDelimiter(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

// paperdollSlots are the equip-slot names an admin command takes, in the
// order of the slot index each names.
var paperdollSlots = [...]string{
	"under", "lear", "rear", "neck", "lfinger", "rfinger", "head", "rhand", "lhand",
	"gloves", "chest", "legs", "feet", "cloak", "face", "hair", "hairall",
}

// PaperdollSlot returns the equip-slot index name names, matched
// case-insensitively.
func PaperdollSlot(name string) (int, bool) {
	for i, slot := range paperdollSlots {
		if strings.EqualFold(slot, name) {
			return i, true
		}
	}
	return 0, false
}

// PaperdollSlotName returns the upper-case name of equip slot index slot.
func PaperdollSlotName(slot int) string {
	if slot < 0 || slot >= len(paperdollSlots) {
		return ""
	}
	return strings.ToUpper(paperdollSlots[slot])
}
