package task

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func TestInventoryUpdatesTickSendsVisibleOwnersAndUpdatesWeight(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Weight: 2, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})

	owner := &inventoryUpdateOwnerStub{visible: true}
	updates := NewInventoryUpdates()
	updates.Add(inv, owner)

	updates.Tick()

	if got, want := owner.sent, [][]itemcontainer.Update{{{ObjectID: 1, TemplateID: 57, Count: 3, State: itemcontainer.UpdateAdded}}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("sent updates = %+v, want %+v", got, want)
	}
	if got := inv.TotalWeight(); got != 6 {
		t.Fatalf("TotalWeight() = %d, want 6", got)
	}
	if got := inv.DrainUpdates(); len(got) != 0 {
		t.Fatalf("DrainUpdates() after send = %+v, want empty", got)
	}
}

func TestInventoryUpdatesTickDropsInvisibleNonTeleportingOwners(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, Stackable: true}})
	inv := itemcontainer.NewPlayerInventory(0x10000001, templates)
	inv.Add(&item.Instance{ObjectID: 1, TemplateID: 57, Count: 3})

	owner := &inventoryUpdateOwnerStub{}
	updates := NewInventoryUpdates()
	updates.Add(inv, owner)

	updates.Tick()

	if len(owner.sent) != 0 {
		t.Fatalf("sent updates = %+v, want none", owner.sent)
	}
	if updates.Contains(inv) {
		t.Fatalf("invisible non-teleporting owner should be removed from the task")
	}
	if got := inv.DrainUpdates(); len(got) != 1 {
		t.Fatalf("DrainUpdates() = %+v, want the pending update to remain queued", got)
	}
}

type inventoryUpdateOwnerStub struct {
	visible     bool
	teleporting bool
	sent        [][]itemcontainer.Update
}

func (o *inventoryUpdateOwnerStub) Visible() bool { return o.visible }

func (o *inventoryUpdateOwnerStub) Teleporting() bool { return o.teleporting }

func (o *inventoryUpdateOwnerStub) SendInventoryUpdate(updates []itemcontainer.Update) {
	o.sent = append(o.sent, slices.Clone(updates))
}
