package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// TestOwnerDiscoversPetOffOwnQueueSendsItemListOnOwnQueue drives the owner's
// discover of their own pet from another goroutine, the way the pet walking
// back into view from its own queue does. The PetItemList must
// still be built and sent on the owner's queue (the send asserts it), so a
// pet-inventory removal drained there afterwards reaches the client behind
// the snapshot that still lists the item, never ahead of it.
func TestOwnerDiscoversPetOffOwnQueueSendsItemListOnOwnQueue(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootOwnerPetOutOfView(t)
	pet.SyncPosition(home)
	queue.Post(func() { pet.PetInventory().DestroyByTemplateID(wolfFoodID, 5) })
	h.srv.Settle(t)
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)

	list, update := -1, -1
	for i, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodePetItemList:
			list = i
		case serverpackets.OpcodePetInventoryUpdate:
			update = i
		}
	}
	if list < 0 || update < 0 || list > update {
		t.Fatalf("frames = opcodes %x, want PetItemList before PetInventoryUpdate", frameOpcodes(frames))
	}
	if n := wire.NewReader(frames[list][1:]).ReadUint16(); n != 1 {
		t.Fatalf("PetItemList item count = %d, want 1 (the food, before its removal)", n)
	}
}

// TestOwnerPetItemListDroppedWhenPetUnsummonedFirst discovers the pet and
// unsummons it in one owner-queue task, so the posted PetItemList runs after
// the PetDelete: it must send nothing rather than list a pet that is gone.
func TestOwnerPetItemListDroppedWhenPetUnsummonedFirst(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootOwnerPetOutOfView(t)

	queue.Post(func() {
		pet.SyncPosition(home)
		pet.Unsummon()
	})
	h.srv.Settle(t)
	requireNoPetItemListAfter(t, drainFrames(t, h.client), serverpackets.OpcodePetDelete)
}

// TestOwnerPetItemListDroppedWhenPetLeftViewFirst discovers the pet and moves
// it back out of the owner's view in one owner-queue task: the posted
// PetItemList must neither follow DeleteObject nor drain the pet's pending
// inventory updates.
func TestOwnerPetItemListDroppedWhenPetLeftViewFirst(t *testing.T) {
	t.Parallel()
	h, pet, queue, home := bootOwnerPetOutOfView(t)
	away := location.Location{X: home.X + 20000, Y: home.Y, Z: home.Z}

	queue.Post(func() {
		pet.PetInventory().DestroyByTemplateID(wolfFoodID, 1)
		pet.SyncPosition(home)
		pet.SyncPosition(away)
	})
	h.srv.Settle(t)
	requireNoPetItemListAfter(t, drainFrames(t, h.client), serverpackets.OpcodeDeleteObject)
	if !pet.PetInventory().HasUpdates() {
		t.Fatal("a dropped PetItemList drained the pet's pending inventory updates")
	}
}

func TestUnsummonSendsPetDeleteWhenOwnerCannotSeePet(t *testing.T) {
	t.Parallel()
	h, pet, _, _ := bootOwnerPetOutOfView(t)
	pet.Unsummon()
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete for out-of-view pet")
}

// bootOwnerPetOutOfView spawns the owner's wolf holding 5 food, then moves it
// 20000 units away on the owner's queue so the owner forgets it. It returns
// the owner's location, where moving the pet brings it back into view.
func bootOwnerPetOutOfView(t *testing.T) (*petWorld, *summon.Actor, *sim.Queue, location.Location) {
	t.Helper()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: wolfFoodID, Count: 5})
	pet, _ := h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, wolfFoodID), 5)
	h.srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, h.client)

	queue := h.srv.PlayerQueue(t, h.ownerID)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	away := location.Location{X: x + 20000, Y: y, Z: z}
	queue.Post(func() { pet.SyncPosition(away) })
	readUntilOpcode(t, h.client, serverpackets.OpcodeDeleteObject, "DeleteObject as the pet leaves view")
	drainUntilQuiet(t, h.client)
	return h, pet, queue, location.Location{X: x, Y: y, Z: z}
}

// requireNoPetItemListAfter fails unless frames hold the departure packet
// and no PetItemList after it.
func requireNoPetItemListAfter(t *testing.T, frames [][]byte, departure byte) {
	t.Helper()
	deleted := false
	for _, frame := range frames {
		switch frame[0] {
		case departure:
			deleted = true
		case serverpackets.OpcodePetItemList:
			if deleted {
				t.Fatalf("frames = opcodes %x, PetItemList after %x", frameOpcodes(frames), departure)
			}
		}
	}
	if !deleted {
		t.Fatalf("frames = opcodes %x, want %x", frameOpcodes(frames), departure)
	}
}
