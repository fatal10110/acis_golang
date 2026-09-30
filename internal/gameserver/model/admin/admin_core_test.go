package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// ---- from admin_test.go ----
func TestNewAccessLevel(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("level", "7")
	set.Set("name", "Admin")
	set.Set("childLevel", "6")
	set.Set("isGM", "true")
	set.Set("allowFixedRes", "true")
	set.Set("allowAltg", "true")

	got, err := NewAccessLevel(set)
	if err != nil {
		t.Fatalf("NewAccessLevel() error: %v", err)
	}
	if got.Level != 7 || got.Name != "Admin" || got.NameColor != "FFFFFF" || got.TitleColor != "FFFF77" || !got.IsGM || got.ChildLevel != 6 || !got.AllowTransaction || !got.GiveDamage {
		t.Fatalf("NewAccessLevel() = %+v", got)
	}
}

func TestNewAdminCommand(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("name", "admin_ann")
	set.Set("accessLevel", "7")
	set.Set("params", "message")
	set.Set("desc", "Broadcast the message, with 'Announcements:' tag.")

	got, err := NewCommand(set)
	if err != nil {
		t.Fatalf("NewCommand() error: %v", err)
	}
	if got.Name != "admin_ann" || got.AccessLevel != 7 || got.Params != "message" {
		t.Fatalf("NewCommand() = %+v", got)
	}
}

func TestNewAnnouncement(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("message", "Server restart soon.")
	set.Set("critical", "true")
	set.Set("auto", "true")
	set.Set("initial_delay", "60")
	set.Set("delay", "300")
	set.Set("limit", "5")

	got, err := NewAnnouncement(set)
	if err != nil {
		t.Fatalf("NewAnnouncement() error: %v", err)
	}
	if got.Message != "Server restart soon." || !got.Critical || !got.Auto || got.InitialDelay != 60 || got.Delay != 300 || got.Limit != 5 {
		t.Fatalf("NewAnnouncement() = %+v", got)
	}

	set = commons.NewStatSet()
	set.Set("message", "")
	got, err = NewAnnouncement(set)
	if err != nil || got.Message != "" {
		t.Fatalf("NewAnnouncement(empty) = %+v, %v", got, err)
	}
}

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
