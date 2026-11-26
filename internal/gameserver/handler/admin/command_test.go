package admin

import (
	"slices"
	"testing"
)

// TestSpaceArgsSplitsOnSpacesOnly pins SpaceArgs to a tokenizer with a
// space delimiter: other whitespace stays inside an argument.
func TestSpaceArgsSplitsOnSpacesOnly(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"admin_info", []string{}},
		{"admin_info  drop 2 ", []string{"drop", "2"}},
		{"admin_info\tdrop", []string{}},
		{"admin_info a\tb", []string{"a\tb"}},
	} {
		if got := SpaceArgs(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("SpaceArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTrimAndDress pins TrimAndDress, which names a private's
// master in //list_spawns: a name longer than the width keeps its first
// width-3 characters and gains "...".
func TestTrimAndDress(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Wolf", "Wolf"},
		{"Exactly Twenty Chars", "Exactly Twenty Chars"},
		{"Lord of the Wolf Pack", "Lord of the Wolf ..."},
		{"", ""},
	} {
		if got := TrimAndDress(tc.in, 20); got != tc.want {
			t.Fatalf("TrimAndDress(%q, 20) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
