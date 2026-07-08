package item

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestNewSummonItem(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "2375")
	set.Set("npcId", "12077")
	set.Set("summonType", "1")

	got, err := NewSummonItem(set)
	if err != nil {
		t.Fatalf("NewSummonItem() error: %v", err)
	}

	want := SummonItem{ItemID: 2375, NPCID: 12077, SummonType: 1}
	if got != want {
		t.Fatalf("NewSummonItem() = %+v, want %+v", got, want)
	}

	set = commons.NewStatSet()
	set.Set("id", "2375")
	if _, err := NewSummonItem(set); err == nil {
		t.Fatal("expected an error for missing npcId/summonType, got nil")
	}
}
