package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewAccessLevel(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("level", "7")
	set.Set("name", "Admin")
	set.Set("nameColor", "CC6600")
	set.Set("titleColor", "CC6600")
	set.Set("childLevel", "6")
	set.Set("isGM", "true")
	set.Set("allowFixedRes", "true")
	set.Set("allowTransaction", "true")
	set.Set("allowAltg", "true")
	set.Set("giveDamage", "true")

	got, err := NewAccessLevel(set)
	if err != nil {
		t.Fatalf("NewAccessLevel() error: %v", err)
	}
	if got.Level != 7 || got.Name != "Admin" || !got.IsGM || got.ChildLevel != 6 {
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
	if _, err := NewAnnouncement(set); err == nil {
		t.Fatal("expected an error for an empty message, got nil")
	}
}
