package network

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// recordingItemPersistence captures what the lazy persistence task writes,
// standing in for the items table.
type recordingItemPersistence struct {
	mu      sync.Mutex
	saved   []item.InstanceState
	deleted []int32
}

func (r *recordingItemPersistence) Save(_ context.Context, inst *item.Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved = append(r.saved, inst.Snapshot())
	return nil
}

func (r *recordingItemPersistence) Delete(_ context.Context, objectID int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, objectID)
	return nil
}

func (r *recordingItemPersistence) savedCount(objectID int32) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, st := range r.saved {
		if st.ObjectID == objectID {
			return st.Count, true
		}
	}
	return 0, false
}

// TestDetachLivePlayerFlushesPendingItems covers the reference's
// ItemContainer.deleteMe: a logging-out player's items leave the pending
// set and are written immediately, rather than waiting for a tick that will
// never see that inventory again.
func TestDetachLivePlayerFlushesPendingItems(t *testing.T) {
	store := &recordingItemPersistence{}
	items := task.NewItemInstances(store, nil, nil, testItemTemplates())

	live := newTestLivePlayer(t, 101, &frameCapture{})
	inv := live.Character.Inventory()
	if inv == nil {
		t.Fatal("test player has no inventory")
	}
	inv.SetItemPersister(items.Add)

	inst := inv.AddNew(item.AdenaID, 500, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}
	if !items.Contains(inst) {
		t.Fatal("added item is not pending persistence")
	}

	gcl := &GameClientLink{itemInstances: items, log: zerolog.Nop()}
	gcl.detachLivePlayer(context.Background(), live)

	count, ok := store.savedCount(0x20000001)
	if !ok {
		t.Fatal("detach did not save the pending item")
	}
	if count != 500 {
		t.Errorf("saved count = %d, want 500", count)
	}
	if items.Contains(inst) {
		t.Error("item still pending after detach")
	}

	// The hook is released with the player, so a later mutation of a
	// detached inventory doesn't re-register it.
	inst.AddCount(1)
	if items.Contains(inst) {
		t.Error("detached inventory still registers mutations")
	}
}
