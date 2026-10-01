package macro

import (
	"reflect"
	"strings"
	"testing"
)

// The expected values below follow the persisted-format contract
// (MacroList.registerMacroInDb / restore): lines "type,d1,d2[,text];", the
// whole cut to 255 chars, read back through java.util.StringTokenizer, which
// skips empty tokens, ignores a line of fewer than three fields, and keeps
// only the fourth field as text.

func TestEncodeCommands(t *testing.T) {
	got := EncodeCommands([]Command{
		{Type: CommandSkill, D1: 1177, D2: 0},
		{Type: CommandAction, D1: 2, D2: 0, Text: ""},
		{Type: CommandShortcut, D1: 1, D2: 5, Text: "/say hi"},
	})
	if want := "1,1177,0;3,2,0;4,1,5,/say hi;"; got != want {
		t.Fatalf("EncodeCommands = %q, want %q", got, want)
	}
}

func TestEncodeCommandsCutsAt255CodeUnits(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := EncodeCommands([]Command{{Type: 1, D1: 2, D2: 3, Text: long}})
	if len(got) != 255 || got != ("1,2,3," + long)[:255] {
		t.Fatalf("EncodeCommands length %d, want the first 255 chars", len(got))
	}
	// A supplementary character counts two code units: 248 + 6 ASCII
	// fields = 254 units, so the pair would end at 256 and is dropped.
	pair := EncodeCommands([]Command{{Type: 1, D1: 2, D2: 3, Text: strings.Repeat("b", 248) + "\U0001F600"}})
	if UTF16Len(pair) != 254 || strings.ContainsRune(pair, '\U0001F600') {
		t.Fatalf("cut through a surrogate pair kept %d units", UTF16Len(pair))
	}
}

func TestDecodeCommands(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Command
	}{
		{"plain lines", "1,1177,0;4,1,5,/say hi;", []Command{
			{Type: 1, D1: 1177, D2: 0},
			{Type: 4, D1: 1, D2: 5, Text: "/say hi"},
		}},
		{"empty pieces skipped", ";;3,,2,,0;;", []Command{{Type: 3, D1: 2, D2: 0}}},
		{"short line skipped and not numbered", "1,2;3,4,5;", []Command{{Type: 3, D1: 4, D2: 5}}},
		{"comma in text keeps only the fourth field", "3,0,0,/say a,b;", []Command{{Type: 3, Text: "/say a"}}},
		{"signed numbers", "+1,-5,0;", []Command{{Type: 1, D1: -5}}},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeCommands(tc.in)
			if err != nil {
				t.Fatalf("DecodeCommands(%q): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DecodeCommands(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
	if _, err := DecodeCommands("1,x,0;"); err == nil {
		t.Fatal("non-numeric d1 decoded without error")
	}
}

// TestRestoreStopsAtMalformedRow pins the restore's failure shape: a
// non-numeric field aborts the read, keeping the macros read before it.
func TestRestoreStopsAtMalformedRow(t *testing.T) {
	list, err := Restore([]Row{
		{ID: 1000, Name: "A", Commands: "1,2,3;", CommandsValid: true},
		{ID: 1001, Name: "B", Commands: "1,z,3;", CommandsValid: true},
		{ID: 1002, Name: "C", Commands: "", CommandsValid: true},
	})
	if err == nil {
		t.Fatal("Restore of a malformed row returned no error")
	}
	if list.Len() != 1 || !list.Has(1000) {
		t.Fatalf("restored %+v, want only macro 1000", list.All())
	}
	if _, err := Restore([]Row{{ID: 5, Name: "N"}}); err == nil {
		t.Fatal("Restore of a NULL command column returned no error")
	}
}

func TestRegisterAllocatesFreeIDsFrom1000AndKeepsOrder(t *testing.T) {
	list := NewList()
	list.Register(Macro{ID: 1001, Name: "taken"})
	a := list.Register(Macro{Name: "a"})
	b := list.Register(Macro{Name: "b"})
	if a.ID != 1000 || b.ID != 1002 {
		t.Fatalf("allocated ids %d and %d, want 1000 and 1002", a.ID, b.ID)
	}
	list.Register(Macro{ID: 1001, Name: "edited"})
	var names []string
	for _, m := range list.All() {
		names = append(names, m.Name)
	}
	if want := []string{"edited", "a", "b"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("order after edit = %v, want %v", names, want)
	}
	if !list.Delete(1000) || list.Delete(1000) || list.Len() != 2 {
		t.Fatal("Delete did not remove macro 1000 exactly once")
	}
	if r1, r2 := list.NextRevision(), list.NextRevision(); r1 != 2 || r2 != 3 {
		t.Fatalf("revisions = %d, %d, want 2, 3", r1, r2)
	}
}

func TestCheck(t *testing.T) {
	list := NewList()
	list.Register(Macro{ID: 1000, Name: "Heal"})
	cases := []struct {
		name string
		m    Macro
		text int
		want Refusal
	}{
		{"accepted", Macro{Name: "Buff"}, 255, Accepted},
		{"command text over 255", Macro{Name: "Buff"}, 256, CommandsTooLong},
		{"no name", Macro{}, 0, NameMissing},
		{"name taken ignoring case", Macro{Name: "hEAL"}, 0, NameTaken},
		{"own name on edit", Macro{ID: 1000, Name: "HEAL"}, 0, Accepted},
		{"description of 32", Macro{Name: "Buff", Description: strings.Repeat("d", 32)}, 0, Accepted},
		{"description of 33", Macro{Name: "Buff", Description: strings.Repeat("d", 33)}, 0, DescriptionTooLong},
	}
	for _, tc := range cases {
		if got := list.Check(tc.m, tc.text); got != tc.want {
			t.Errorf("%s: Check = %d, want %d", tc.name, got, tc.want)
		}
	}
	for i := range 24 {
		list.Register(Macro{Name: string(rune('a' + i))})
	}
	if got := list.Check(Macro{Name: "more"}, 0); got != TooMany {
		t.Fatalf("Check with 25 macros = %d, want TooMany", got)
	}
}
