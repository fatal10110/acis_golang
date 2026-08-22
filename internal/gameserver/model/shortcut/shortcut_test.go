package shortcut

import "testing"

func TestListRegisterReplacesSlotAndOrdersByPageSlot(t *testing.T) {
	list := NewList([]Shortcut{
		{Slot: 5, Page: 0, Type: Action, ID: 5, Level: -1, CharacterType: 1},
		{Slot: 1, Page: 1, Type: Item, ID: 57, Level: -1, CharacterType: 1},
	})

	list.Register(Shortcut{Slot: 5, Page: 0, Type: Skill, ID: 248, Level: 1, CharacterType: 1})
	got := list.All()

	want := []Shortcut{
		{Slot: 5, Page: 0, Type: Skill, ID: 248, Level: 1, CharacterType: 1},
		{Slot: 1, Page: 1, Type: Item, ID: 57, Level: -1, CharacterType: 1},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("All() = %+v, want %+v", got, want)
	}
}

func TestListDeleteRemovesSlot(t *testing.T) {
	list := NewList([]Shortcut{{Slot: 3, Page: 1, Type: Action, ID: 2, Level: -1, CharacterType: 1}})

	if !list.Delete(3, 1) {
		t.Fatal("Delete() = false, want true")
	}
	if got := list.All(); len(got) != 0 {
		t.Fatalf("All() after delete = %+v, want empty", got)
	}
}

func TestNewRegistrationValidatesTypePageAndSkillLevel(t *testing.T) {
	skillLevels := func(id int32) int {
		if id == 248 {
			return 3
		}
		return 0
	}
	hasItem := func(id int32) bool { return id == 57 }

	sc, ok := NewRegistration(3, 1, Skill, 248, 1, skillLevels, hasItem)
	if !ok {
		t.Fatal("NewRegistration returned false for known skill")
	}
	if sc != (Shortcut{Slot: 3, Page: 1, Type: Skill, ID: 248, Level: 3, CharacterType: 1, SharedReuseGroup: -1}) {
		t.Fatalf("NewRegistration skill = %+v, want skill level 3", sc)
	}

	sc, ok = NewRegistration(4, 1, Item, 57, 1, nil, hasItem)
	if !ok {
		t.Fatal("NewRegistration returned false for item shortcut")
	}
	if sc.Level != -1 {
		t.Fatalf("item shortcut level = %d, want -1", sc.Level)
	}

	for _, tt := range []struct {
		name string
		page int32
		typ  Type
		id   int32
	}{
		{"negative page", -1, Item, 57},
		{"high page", 11, Item, 57},
		{"bad type", 0, None, 57},
		{"unknown skill", 0, Skill, 999},
		{"item not in inventory", 0, Item, 58},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := NewRegistration(1, tt.page, tt.typ, tt.id, 1, skillLevels, hasItem); ok {
				t.Fatal("NewRegistration returned true, want false")
			}
		})
	}

	if _, ok := NewRegistration(1, 0, Item, 57, 1, skillLevels, nil); ok {
		t.Fatal("NewRegistration returned true for item registration with nil hasItem, want false")
	}
}

func TestListRefreshSkillLevelUpdatesOnlyMatchingSkillShortcuts(t *testing.T) {
	list := NewList([]Shortcut{
		{Slot: 0, Page: 0, Type: Skill, ID: 248, Level: 1, CharacterType: 1},
		{Slot: 1, Page: 0, Type: Skill, ID: 248, Level: 1, CharacterType: 1},
		{Slot: 2, Page: 0, Type: Skill, ID: 999, Level: 1, CharacterType: 1},
		{Slot: 3, Page: 0, Type: Item, ID: 248, Level: -1, CharacterType: 1},
	})

	got := list.RefreshSkillLevel(248, 2)

	want := []Shortcut{
		{Slot: 0, Page: 0, Type: Skill, ID: 248, Level: 2, CharacterType: 1},
		{Slot: 1, Page: 0, Type: Skill, ID: 248, Level: 2, CharacterType: 1},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RefreshSkillLevel() = %+v, want %+v", got, want)
	}

	all := list.All()
	if all[2].Level != 1 || all[3].Level != -1 {
		t.Fatalf("unrelated shortcuts mutated: %+v", all)
	}
}

func TestTutorialBookShortcutUsesGrantedInstanceObjectID(t *testing.T) {
	sc := TutorialBookShortcut(0x10000042)
	want := Shortcut{Slot: 11, Page: 0, Type: Item, ID: 0x10000042, Level: -1, CharacterType: 1, SharedReuseGroup: -1}
	if sc != want {
		t.Fatalf("TutorialBookShortcut() = %+v, want %+v", sc, want)
	}
}

func TestAutoGetSkillShortcutsMapsHardcodedIDsToSlots(t *testing.T) {
	got := AutoGetSkillShortcuts(map[int32]int32{1177: 1, 1216: 1, 9999: 1})

	byID := make(map[int32]Shortcut)
	for _, sc := range got {
		byID[sc.ID] = sc
	}
	if len(got) != 2 {
		t.Fatalf("AutoGetSkillShortcuts() = %+v, want 2 entries (unknown id 9999 dropped)", got)
	}
	if sc := byID[1177]; sc != (Shortcut{Slot: 1, Page: 0, Type: Skill, ID: 1177, Level: 1, CharacterType: 1, SharedReuseGroup: -1}) {
		t.Errorf("skill 1177 shortcut = %+v, want slot 1", sc)
	}
	if sc := byID[1216]; sc != (Shortcut{Slot: 9, Page: 0, Type: Skill, ID: 1216, Level: 1, CharacterType: 1, SharedReuseGroup: -1}) {
		t.Errorf("skill 1216 shortcut = %+v, want slot 9", sc)
	}
}

func TestRestoreItemShortcutsDropsStaleItemDropsSharedReuseGroup(t *testing.T) {
	shortcuts := []Shortcut{
		{Slot: 0, Page: 0, Type: Item, ID: 100, Level: -1, CharacterType: 1, SharedReuseGroup: -1}, // consumed, gone
		{Slot: 1, Page: 0, Type: Item, ID: 200, Level: -1, CharacterType: 1, SharedReuseGroup: -1}, // present, etc item group 4
		{Slot: 2, Page: 0, Type: Action, ID: 5, Level: -1, CharacterType: 1, SharedReuseGroup: -1}, // non-item, untouched
	}

	lookup := func(objectID int32) (int32, bool) {
		if objectID == 200 {
			return 4, true
		}
		return 0, false
	}

	got := RestoreItemShortcuts(shortcuts, lookup)

	want := []Shortcut{
		{Slot: 1, Page: 0, Type: Item, ID: 200, Level: -1, CharacterType: 1, SharedReuseGroup: 4},
		{Slot: 2, Page: 0, Type: Action, ID: 5, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RestoreItemShortcuts() = %+v, want %+v", got, want)
	}
}

func TestTypeStringsRoundTrip(t *testing.T) {
	for _, typ := range []Type{Item, Skill, Action, Macro, Recipe} {
		got, ok := ParseType(typ.String())
		if !ok || got != typ {
			t.Fatalf("ParseType(%q) = %v, %v; want %v, true", typ.String(), got, ok, typ)
		}
	}
}
