package item

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestParseDropKind(t *testing.T) {
	cases := []struct {
		in   string
		want DropKind
	}{
		{"SPOIL", DropSpoil},
		{"CURRENCY", DropCurrency},
		{"DROP", DropNormal},
		{"HERB", DropHerb},
	}
	for _, c := range cases {
		got, err := ParseDropKind(c.in)
		if err != nil {
			t.Fatalf("ParseDropKind(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseDropKind(%q) = %v, want %v", c.in, got, c.want)
		}
	}

	if _, err := ParseDropKind("BOGUS"); err == nil {
		t.Fatal("ParseDropKind(\"BOGUS\") error = nil, want error")
	}
}

func TestNewDrop(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("itemid", "8600")
	set.Set("min", "1")
	set.Set("max", "3")
	set.Set("chance", "55.0")

	got, err := NewDrop(set)
	if err != nil {
		t.Fatalf("NewDrop() error: %v", err)
	}
	want := Drop{ItemID: 8600, Min: 1, Max: 3, Chance: 55.0}
	if got != want {
		t.Fatalf("NewDrop() = %+v, want %+v", got, want)
	}
}

func TestNewDropCategory(t *testing.T) {
	t.Run("explicit chance", func(t *testing.T) {
		set := commons.NewStatSet()
		set.Set("type", "HERB")
		set.Set("chance", "42.0")

		got, err := NewDropCategory(set, nil)
		if err != nil {
			t.Fatalf("NewDropCategory() error: %v", err)
		}
		if got.Kind != DropHerb || got.Chance != 42.0 {
			t.Fatalf("NewDropCategory() = %+v", got)
		}
	})

	t.Run("chance defaults to 100", func(t *testing.T) {
		set := commons.NewStatSet()
		set.Set("type", "DROP")

		got, err := NewDropCategory(set, nil)
		if err != nil {
			t.Fatalf("NewDropCategory() error: %v", err)
		}
		if got.Chance != 100.0 {
			t.Fatalf("NewDropCategory() chance = %v, want 100", got.Chance)
		}
	})

	t.Run("missing type is an error", func(t *testing.T) {
		set := commons.NewStatSet()
		if _, err := NewDropCategory(set, nil); err == nil {
			t.Fatal("NewDropCategory() error = nil, want error")
		}
	})
}
