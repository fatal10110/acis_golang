package admin

import "testing"

// ---- from admin_test.go ----
// TestResolveAccessLevel pins the lookup a character's persisted access level
// goes through at login: any negative level reads the -1 entry, an undefined
// level falls back to the user level 0, and a missing table resolves to the
// attribute defaults (transactions and damage allowed, no GM rights).
func TestResolveAccessLevel(t *testing.T) {
	levels := []AccessLevel{
		{Level: -1, Name: "Banned"},
		{Level: 0, Name: "User", AllowTransaction: true, GiveDamage: true},
		{Level: 2, Name: "Test GM", AllowFixedRes: true},
	}
	data, err := NewData(levels, nil)
	if err != nil {
		t.Fatalf("NewData() error: %v", err)
	}
	for _, tc := range []struct {
		level int
		want  string
	}{
		{-100, "Banned"},
		{-1, "Banned"},
		{0, "User"},
		{2, "Test GM"},
		{9, "User"},
	} {
		if got := data.Resolve(tc.level); got.Name != tc.want {
			t.Errorf("Resolve(%d) = %q, want %q", tc.level, got.Name, tc.want)
		}
	}
	if got := data.Resolve(2); got.AllowTransaction || got.GiveDamage || !got.AllowFixedRes {
		t.Errorf("Resolve(2) = %+v, want the Test GM entry's flags", got)
	}

	var none *Data
	if got := none.Resolve(5); !got.AllowTransaction || !got.GiveDamage || got.IsGM || got.AllowFixedRes {
		t.Errorf("nil table Resolve(5) = %+v, want the attribute defaults", got)
	}
	noUser, err := NewData([]AccessLevel{{Level: 8, Name: "Master", IsGM: true}}, nil)
	if err != nil {
		t.Fatalf("NewData() error: %v", err)
	}
	if got := noUser.Resolve(3); !got.AllowTransaction || got.IsGM {
		t.Errorf("table without level 0 Resolve(3) = %+v, want the attribute defaults", got)
	}
}
