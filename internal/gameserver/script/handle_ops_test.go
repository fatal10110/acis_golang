package script

import "testing"

// TestNpcStringFillsParamsInOrder pins NpcStringId.getMessage: each param
// replaces the next placeholder, a text short of params keeps the rest, and
// a text with no placeholder is returned as is.
func TestNpcStringFillsParamsInOrder(t *testing.T) {
	for _, tc := range []struct {
		id     int32
		params []any
		want   string
	}{
		{31603, nil, "Why do you oppress us so?"},
		{1, []any{"Gremlin", 7}, "Hello! I am Gremlin. Ha-ha, you are 7? Hee hee hee hee"},
		{1, []any{"Gremlin"}, "Hello! I am Gremlin. Ha-ha, you are %s? Hee hee hee hee"},
		{1, nil, "Hello! I am %s. Ha-ha, you are %s? Hee hee hee hee"},
	} {
		if got := NpcString(tc.id, tc.params...); got != tc.want {
			t.Errorf("NpcString(%d, %v) = %q, want %q", tc.id, tc.params, got, tc.want)
		}
	}
}

// TestNpcStringUnknownIDPanics: an id with no text aborts the invocation.
func TestNpcStringUnknownIDPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NpcString(0) did not panic")
		}
	}()
	NpcString(0)
}
