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

	sc, ok := NewRegistration(3, 1, Skill, 248, 1, skillLevels)
	if !ok {
		t.Fatal("NewRegistration returned false for known skill")
	}
	if sc != (Shortcut{Slot: 3, Page: 1, Type: Skill, ID: 248, Level: 3, CharacterType: 1}) {
		t.Fatalf("NewRegistration skill = %+v, want skill level 3", sc)
	}

	sc, ok = NewRegistration(4, 1, Item, 57, 1, nil)
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := NewRegistration(1, tt.page, tt.typ, tt.id, 1, skillLevels); ok {
				t.Fatal("NewRegistration returned true, want false")
			}
		})
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
