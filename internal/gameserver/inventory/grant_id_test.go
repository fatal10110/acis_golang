package inventory

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// TestReserveGrantTakesNoIDForAHeldStack: a grant onto a held stackable
// reserves no id, and should that stack go before the grant lands, the
// grant takes a late one instead of failing.
func TestReserveGrantTakesNoIDForAHeldStack(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, testTemplates())
	adena := inv.AddNew(item.AdenaID, 100, 500)
	ids := &testIDs{next: 900}
	s := NewService(ids)

	grant, ok, err := s.reserveGrant(inv, item.AdenaID)
	if err != nil || !ok {
		t.Fatalf("reserveGrant = %v, %v", ok, err)
	}
	if ids.next != 900 {
		t.Fatalf("reserveGrant onto held adena took id %d, want none", ids.next)
	}
	if inv.DestroyItem(adena, 100) == nil {
		t.Fatal("could not destroy the held adena")
	}
	got, err := inv.AddCreated(item.AdenaID, 30, grant.next)
	if err != nil || got == nil || got.ObjectID != 901 || got.Count != 30 {
		t.Fatalf("grant after the stack went = %+v, %v, want 30 adena under late id 901", got, err)
	}

	grant, ok, err = s.reserveGrant(inv, 30)
	if err != nil || !ok || grant.id != 902 {
		t.Fatalf("reserveGrant of a weapon = %+v, %v, %v, want reserved id 902", grant, ok, err)
	}
	if id, err := grant.next(); err != nil || id != 902 || ids.next != 902 {
		t.Fatalf("grant.next = %d, %v (allocator at %d), want the reserved 902", id, err, ids.next)
	}
}
