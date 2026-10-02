package network

import "testing"

// TestTrimAndDress pins StringUtil.trimAndDress, which names a private's
// master in //list_spawns: a name longer than the width keeps its first
// width-3 characters and gains "...".
func TestTrimAndDress(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Wolf", "Wolf"},
		{"Exactly Twenty Chars", "Exactly Twenty Chars"},
		{"Lord of the Wolf Pack", "Lord of the Wolf ..."},
		{"", ""},
	} {
		if got := trimAndDress(tc.in, 20); got != tc.want {
			t.Fatalf("trimAndDress(%q, 20) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestCommandWord pins //help's name.substring(6): the command word a game
// master types after "//".
func TestCommandWord(t *testing.T) {
	for in, want := range map[string]string{"admin_help": "help", "admin_": "", "adm": ""} {
		if got := commandWord(in); got != want {
			t.Fatalf("commandWord(%q) = %q, want %q", in, got, want)
		}
	}
}
