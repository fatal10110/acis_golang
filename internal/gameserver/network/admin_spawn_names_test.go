package network

import "testing"

// TestCommandWord pins //help's name.substring(6): the command word a game
// master types after "//".
func TestCommandWord(t *testing.T) {
	for in, want := range map[string]string{"admin_help": "help", "admin_": "", "adm": ""} {
		if got := commandWord(in); got != want {
			t.Fatalf("commandWord(%q) = %q, want %q", in, got, want)
		}
	}
}
