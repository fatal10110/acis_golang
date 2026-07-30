package network

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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

// TestFlushItemPersistenceKeepsItemsPendingOnFailure pins the recovery
// path: a flush that fails partway leaves its items pending, so the next
// tick or the shutdown flush retries them instead of dropping them.
func TestFlushItemPersistenceKeepsItemsPendingOnFailure(t *testing.T) {
	store := &failingItemPersistence{}
	items := task.NewItemInstances(store, nil, nil, testItemTemplates())

	live := newTestLivePlayer(t, 102, &frameCapture{})
	inv := live.Character.Inventory()
	if inv == nil {
		t.Fatal("test player has no inventory")
	}
	inv.SetItemPersister(items.Add)
	inst := inv.AddNew(item.AdenaID, 500, 0x20000001)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}

	gcl := &GameClientLink{itemInstances: items, log: zerolog.Nop()}
	gcl.flushItemPersistence(context.Background(), inv)

	if !items.Contains(inst) {
		t.Error("a failed flush dropped the item from the pending set")
	}
}

// failingItemPersistence rejects every write, standing in for a database
// that is unreachable or a deadline that expired mid-flush.
type failingItemPersistence struct{}

func (failingItemPersistence) Save(context.Context, *item.Instance) error {
	return errors.New("save failed")
}

func (failingItemPersistence) Delete(context.Context, int32) error {
	return errors.New("delete failed")
}

// TestUnsummonFlushesPetInventoryPersistence covers the reference's
// PetInstance.deleteMe calling its inventory's deleteMe: a returned pet's
// container goes away, so its items must be written and unregistered right
// there rather than lingering in the pending set behind a despawned pet.
func TestUnsummonFlushesPetInventoryPersistence(t *testing.T) {
	store := &recordingItemPersistence{}
	items := task.NewItemInstances(store, nil, nil, petTestTemplates())

	state := world.New()
	live := newTestLivePlayer(t, 103, &frameCapture{})
	state.Spawn(live, 0, 0, 0, 0)

	petInv := itemcontainer.NewPetInventory(0x20000001, petTestTemplates())
	pet := summon.NewPet(summon.PetConfig{
		ObjectID: 0x20000001, Owner: live, Level: 1,
		Inventory: petInv, Fed: 100, MaxMeal: 100,
	})
	summon.SpawnBesideOwner(state, pet, live, location.Location{X: 10})

	petInv.SetItemPersister(items.Add)
	inst := petInv.AddNew(item.AdenaID, 25, 0x20000002)
	if inst == nil {
		t.Fatal("AddNew() = nil")
	}

	gcl := &GameClientLink{world: state, itemInstances: items, log: zerolog.Nop()}
	// Action id 19 is the pet-return shortcut.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 19}) {
		t.Fatal("handleSummonActionUse returned false for the pet-return command")
	}

	if _, ok := store.savedCount(0x20000002); !ok {
		t.Fatal("unsummon did not save the pet inventory's pending item")
	}
	if items.Contains(inst) {
		t.Error("pet item still pending after unsummon")
	}

	inst.AddCount(1)
	if items.Contains(inst) {
		t.Error("unsummoned pet inventory still registers mutations")
	}
}
