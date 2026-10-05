package npc

import "testing"

// TestParseSubclassCommandReadsUnicodeDigits pins the subclass command to
// the reference's VillageMaster, which reads its choice and numbers with
// Integer.parseInt over String.substring: any Basic Multilingual Plane
// decimal digit reads as its value (a Java probe on OpenJDK 21.0.11,
// recorded in #3091, prints Integer.parseInt("１２") and
// Integer.parseInt("١٢") as 12), and the fixed offsets count UTF-16 units,
// so a wide character ahead of the choice moves it by one character, not by
// its UTF-8 length. A digit outside the plane is two units, and its high
// half alone is no digit.
func TestParseSubclassCommandReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, command string
		want          SubclassCommand
	}{
		{"fullwidth choice alone", "Subclass １", SubclassCommand{Choice: 1}},
		{"arabic-indic class id", "Subclass 4 ١٢", SubclassCommand{Choice: 4, One: 12}},
		{"fullwidth and arabic-indic pair", "Subclass ７ ８８ ٩٠", SubclassCommand{Choice: 7, One: 88, Two: 90}},
		{"devanagari second number", "Subclass 6 2 ९", SubclassCommand{Choice: 6, One: 2, Two: 9}},
		// The ideographic space is one unit, so the choice is still the
		// tenth character and the class id the twelfth.
		{"wide character ahead of the choice", "Subclass　１ 5", SubclassCommand{Choice: 1, One: 5}},
		{"supplementary digit choice", "Subclass \U0001D7CF", SubclassCommand{}},
		{"fullwidth letter choice", "Subclass ａ", SubclassCommand{}},
		// A bad class id stops reading there, leaving the choice.
		{"fullwidth letter class id", "Subclass 4 ａ 2", SubclassCommand{Choice: 4}},
		{"ascii", "Subclass 5 3 88", SubclassCommand{Choice: 5, One: 3, Two: 88}},
	} {
		if got := ParseSubclassCommand(tc.command); got != tc.want {
			t.Errorf("%s: ParseSubclassCommand(%q) = %+v, want %+v", tc.name, tc.command, got, tc.want)
		}
	}
}
