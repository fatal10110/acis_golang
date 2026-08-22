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
	// SharedReuseGroup mirrors Java's Shortcut._sharedReuseGroup, which
	// defaults to -1 and is only ever populated for an ITEM shortcut on
	// restore (ShortcutList.java:173-209) — never persisted, always
	// recomputed from the live inventory. See RestoreItemShortcuts.
	SharedReuseGroup int32
}

// NewRegistration validates and builds a client shortcut registration.
// hasItem mirrors ShortcutList.addShortcut's ITEM branch
// (ShortcutList.java:62-98): an ITEM registration for an objectId not in the
// player's live inventory is dropped rather than persisted.
func NewRegistration(slot, page int32, typ Type, id, characterType int32, skillLevel func(int32) int, hasItem func(int32) bool) (Shortcut, bool) {
	if page < 0 || page > MaxRegistrationPage || typ < Item || typ > Recipe {
		return Shortcut{}, false
	}
	level := int32(-1)
	switch typ {
	case Skill:
		if skillLevel == nil {
			return Shortcut{}, false
		}
		level = int32(skillLevel(id))
		if level <= 0 {
			return Shortcut{}, false
		}
	case Item:
		if hasItem == nil || !hasItem(id) {
			return Shortcut{}, false
		}
	}
	return Shortcut{
		Slot:             slot,
		Page:             page,
		Type:             typ,
		ID:               id,
		Level:            level,
		CharacterType:    characterType,
		SharedReuseGroup: -1,
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
		{Slot: 0, Page: 0, Type: Action, ID: 2, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
		{Slot: 3, Page: 0, Type: Action, ID: 5, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
		{Slot: 10, Page: 0, Type: Action, ID: 0, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
	}
}

// TutorialBookItemID is the reference-hardcoded template id of the tutorial
// guide granted to every base profession (RequestCharacterCreate.java:150).
const TutorialBookItemID = 5588

// TutorialBookShortcut returns the slot-11 ITEM shortcut for the tutorial
// book granted at character creation, keyed on the granted instance's own
// objectID rather than the template id — the client cannot resolve an ITEM
// shortcut any other way (RequestCharacterCreate.java:149-151).
func TutorialBookShortcut(objectID int32) Shortcut {
	return Shortcut{Slot: 11, Page: 0, Type: Item, ID: objectID, Level: -1, CharacterType: 1, SharedReuseGroup: -1}
}

// Auto-get skill ids RequestCharacterCreate.java:157-164 hardcodes to a
// starter shortcut slot: 1001 (orc mystics) and 1177 (other mystics) share
// slot 1, 1216 shares slot 9. Any other auto-get skill id gets no shortcut.
const (
	autoGetSkillOrcMystic   int32 = 1001
	autoGetSkillOtherMystic int32 = 1177
	autoGetSkillSlot9       int32 = 1216
)

// AutoGetSkillShortcuts returns the default shortcut-bar entries for a new
// character's free (auto-get) skills, given their id/level pairs. It mirrors
// RequestCharacterCreate.java:157-164, which hardcodes the shortcut level to
// 1 rather than the granted skill's own level.
func AutoGetSkillShortcuts(autoGet map[int32]int32) []Shortcut {
	var out []Shortcut
	for id := range autoGet {
		switch id {
		case autoGetSkillOrcMystic, autoGetSkillOtherMystic:
			out = append(out, Shortcut{Slot: 1, Page: 0, Type: Skill, ID: id, Level: 1, CharacterType: 1, SharedReuseGroup: -1})
		case autoGetSkillSlot9:
			out = append(out, Shortcut{Slot: 9, Page: 0, Type: Skill, ID: id, Level: 1, CharacterType: 1, SharedReuseGroup: -1})
		}
	}
	return out
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

// RefreshSkillLevel sets level on every SKILL shortcut referencing skillID
// and returns the updated entries, ordered by page then slot. It mirrors
// Java's ShortcutList.refreshShortcuts(predicate, level) for the skill-id
// predicate: callers use it to re-point shortcut slots at a skill's new
// level after a grant or upgrade.
func (l *List) RefreshSkillLevel(skillID, level int32) []Shortcut {
	if l == nil || l.bySlot == nil {
		return nil
	}
	var out []Shortcut
	for key, sc := range l.bySlot {
		if sc.Type != Skill || sc.ID != skillID {
			continue
		}
		sc.Level = level
		l.bySlot[key] = sc
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		return out[i].Slot < out[j].Slot
	})
	return out
}

// ItemLookup resolves an ITEM shortcut's target objectID against the live
// inventory. ok is false when the inventory no longer holds the item; group
// is its shared reuse group (-1 if it isn't an etc item), meaningful only
// when ok is true.
type ItemLookup func(objectID int32) (group int32, ok bool)

// RestoreItemShortcuts mirrors ShortcutList.restore()'s per-row ITEM handling
// (ShortcutList.java:173-209): an ITEM shortcut whose item no longer exists
// in the owner's inventory is dropped, and every surviving ITEM shortcut has
// its SharedReuseGroup populated via lookup. Other shortcut types pass
// through unchanged.
func RestoreItemShortcuts(shortcuts []Shortcut, lookup ItemLookup) []Shortcut {
	out := make([]Shortcut, 0, len(shortcuts))
	for _, sc := range shortcuts {
		if sc.Type == Item {
			group, ok := lookup(sc.ID)
			if !ok {
				continue
			}
			sc.SharedReuseGroup = group
		}
		out = append(out, sc)
	}
	return out
}

func slotKey(slot, page int32) int32 {
	return slot + page*12
}
