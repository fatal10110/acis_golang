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

func TestTypeStringsRoundTrip(t *testing.T) {
	for _, typ := range []Type{Item, Skill, Action, Macro, Recipe} {
		got, ok := ParseType(typ.String())
		if !ok || got != typ {
			t.Fatalf("ParseType(%q) = %v, %v; want %v, true", typ.String(), got, ok, typ)
		}
	}
}
