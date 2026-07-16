// Package shortcut stores a player's client shortcut bar entries.
package shortcut

import "sort"

// Type is the client shortcut category ordinal.
type Type int32

// Shortcut types.
const (
	None Type = iota
	Item
	Skill
	Action
	Macro
	Recipe
)

const (
	// MaxRegistrationPage is the highest shortcut page accepted for registration.
	MaxRegistrationPage int32 = 10
	// MaxDeletePage is the highest shortcut page accepted for deletion.
	MaxDeletePage int32 = 9
)

var typeNames = [...]string{"NONE", "ITEM", "SKILL", "ACTION", "MACRO", "RECIPE"}

// String returns the database representation for t.
func (t Type) String() string {
	if t < None || int(t) >= len(typeNames) {
		return typeNames[None]
	}
	return typeNames[t]
}

// ParseType parses a database shortcut type.
func ParseType(s string) (Type, bool) {
	for i, name := range typeNames {
		if name == s {
			return Type(i), true
		}
	}
	return None, false
}

// Shortcut is one client shortcut bar entry.
type Shortcut struct {
	Slot          int32
	Page          int32
	Type          Type
	ID            int32
	Level         int32
	CharacterType int32
}

// NewRegistration validates and builds a client shortcut registration.
func NewRegistration(slot, page int32, typ Type, id, characterType int32, skillLevel func(int32) int) (Shortcut, bool) {
	if page < 0 || page > MaxRegistrationPage || typ < Item || typ > Recipe {
		return Shortcut{}, false
	}
	level := int32(-1)
	if typ == Skill {
		if skillLevel == nil {
			return Shortcut{}, false
		}
		level = int32(skillLevel(id))
		if level <= 0 {
			return Shortcut{}, false
		}
	}
	return Shortcut{
		Slot:          slot,
		Page:          page,
		Type:          typ,
		ID:            id,
		Level:         level,
		CharacterType: characterType,
	}, true
}

// ValidDeletePage reports whether page is accepted for shortcut deletion.
func ValidDeletePage(page int32) bool {
	return page >= 0 && page <= MaxDeletePage
}

// List is an in-memory shortcut bar. It is owned by one live player
// goroutine; callers must serialize access the same way they serialize
// player packet handling.
type List struct {
	bySlot map[int32]Shortcut
}

// NewList returns a shortcut list seeded with shortcuts.
func NewList(shortcuts []Shortcut) *List {
	l := &List{bySlot: make(map[int32]Shortcut, len(shortcuts))}
	for _, shortcut := range shortcuts {
		l.Register(shortcut)
	}
	return l
}

// Starter returns the default shortcuts granted to a new character.
func Starter() []Shortcut {
	return []Shortcut{
		{Slot: 0, Page: 0, Type: Action, ID: 2, Level: -1, CharacterType: 1},
		{Slot: 3, Page: 0, Type: Action, ID: 5, Level: -1, CharacterType: 1},
		{Slot: 10, Page: 0, Type: Action, ID: 0, Level: -1, CharacterType: 1},
	}
}

// Register adds or replaces shortcut.
func (l *List) Register(shortcut Shortcut) {
	if l.bySlot == nil {
		l.bySlot = make(map[int32]Shortcut)
	}
	l.bySlot[slotKey(shortcut.Slot, shortcut.Page)] = shortcut
}

// Has reports whether a shortcut exists at slot and page.
func (l *List) Has(slot, page int32) bool {
	if l == nil || l.bySlot == nil {
		return false
	}
	_, ok := l.bySlot[slotKey(slot, page)]
	return ok
}

// Delete removes one shortcut by slot and page.
func (l *List) Delete(slot, page int32) bool {
	if l == nil || l.bySlot == nil {
		return false
	}
	key := slotKey(slot, page)
	if _, ok := l.bySlot[key]; !ok {
		return false
	}
	delete(l.bySlot, key)
	return true
}

// All returns shortcuts ordered by page, then slot.
func (l *List) All() []Shortcut {
	if l == nil || len(l.bySlot) == 0 {
		return nil
	}
	out := make([]Shortcut, 0, len(l.bySlot))
	for _, shortcut := range l.bySlot {
		out = append(out, shortcut)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		return out[i].Slot < out[j].Slot
	})
	return out
}

func slotKey(slot, page int32) int32 {
	return slot + page*12
}
