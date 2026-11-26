package itemcontainer

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestReserveIDsHandsOutReservedThenLateIDs(t *testing.T) {
	next := int32(100)
	alloc := func() (int32, error) { next++; return next, nil }
	r, err := ReserveIDs(2, alloc)
	if err != nil {
		t.Fatal(err)
	}
	if next != 102 {
		t.Fatalf("ReserveIDs(2) took ids up to %d, want 102", next)
	}
	for _, want := range []int32{101, 102, 103} {
		if got, err := r.Next(); err != nil || got != want {
			t.Fatalf("Next() = %d, %v, want %d", got, err, want)
		}
	}
}

func TestReserveIDsFailsWhenIDsRunOut(t *testing.T) {
	exhausted := errors.New("exhausted")
	left := 1
	alloc := func() (int32, error) {
		if left == 0 {
			return 0, exhausted
		}
		left--
		return 7, nil
	}
	if _, err := ReserveIDs(2, alloc); !errors.Is(err, exhausted) {
		t.Fatalf("ReserveIDs(2) with one id left = %v, want the allocator's error", err)
	}
}

func TestInventoryPayAdena(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := NewPlayerInventory(0x10000001, templates)

	if paid, ok := inv.PayAdena(0); !ok || paid != nil {
		t.Fatalf("PayAdena(0) with no adena = %v, %v, want nil, true", paid, ok)
	}
	if paid, ok := inv.PayAdena(1); ok || paid != nil {
		t.Fatalf("PayAdena(1) with no adena = %v, %v, want nil, false", paid, ok)
	}
	stack := inv.AddNew(item.AdenaID, 100, 0x20000001)
	if paid, ok := inv.PayAdena(101); ok || paid != nil || stack.Count != 100 {
		t.Fatalf("PayAdena(101) of 100 = %v, %v, count %d, want nil, false, 100", paid, ok, stack.Count)
	}
	if paid, ok := inv.PayAdena(-5); !ok || paid != nil || stack.Count != 100 {
		t.Fatalf("PayAdena(-5) = %v, %v, count %d, want nil, true, 100", paid, ok, stack.Count)
	}
	if paid, ok := inv.PayAdena(40); !ok || paid != stack || stack.Count != 60 {
		t.Fatalf("PayAdena(40) of 100 = %v, %v, count %d, want the stack, true, 60", paid, ok, stack.Count)
	}
	if paid, ok := inv.PayAdena(60); !ok || paid != stack || inv.ItemByObjectID(stack.ObjectID) != nil {
		t.Fatalf("PayAdena(60) of 60 = %v, %v, want the emptied stack gone", paid, ok)
	}
}
