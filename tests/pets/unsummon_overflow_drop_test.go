package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestUnsummonOverflowDropsAroundPetAsPlayerDrop pins where and for how long
// a returning pet's item the owner has no room for lands
// (PetInventory.deleteMe -> ItemInstance.dropMe(pet, 70)): at a random spot
// within 70 of the pet on each axis, which geodata validates, and with the
// player-dropped multiplier on its destroy delay, since the pet is a
// playable dropper (ItemsOnGroundTaskManager.add).
func TestUnsummonOverflowDropsAroundPetAsPlayerDrop(t *testing.T) {
	t.Parallel()
	const multiplier = 3
	h := bootOwnerWithCollarOpts(t,
		[]gameservertest.Option{gameservertest.WithPlayerDroppedMultiplier(multiplier)},
		seedItem{TemplateID: item.AdenaID, Count: 100},
		seedItem{TemplateID: wolfFoodID, Count: 2},
	)
	adenaID := h.seededItem(t, item.AdenaID)
	foodID := h.seededItem(t, wolfFoodID)

	pet, _ := h.spawnWolf(t)
	h.giveToPet(t, adenaID, 100)
	h.giveToPet(t, foodID, 2)
	// The owner keeps only the collar: both returning stacks overflow.
	h.srv.SetInventorySlotLimit(t, h.ownerID, 1)
	px, py, _ := pet.Position()

	h.returnPet(t)

	dropped := map[int32]bool{adenaID: false, foodID: false}
	atPet := 0
	for _, s := range h.srv.GroundItems.Snapshots(nil) {
		if _, ok := dropped[s.ObjectID]; !ok {
			continue
		}
		dropped[s.ObjectID] = true
		if s.X < px-70 || s.X > px+70 || s.Y < py-70 || s.Y > py+70 {
			t.Errorf("item %d dropped at (%d,%d), want within 70 of the pet at (%d,%d)", s.ObjectID, s.X, s.Y, px, py)
		}
		if s.X == px && s.Y == py {
			atPet++
		}
		// One hour plain delay, three with the multiplier, less the moments
		// since the drop.
		if left := time.Duration(s.TimeLeftMillis) * time.Millisecond; left <= 2*time.Hour || left > multiplier*time.Hour {
			t.Errorf("item %d destroy delay left = %v, want the player-dropped %dh", s.ObjectID, left, multiplier)
		}
	}
	for id, ok := range dropped {
		if !ok {
			t.Fatalf("item %d is not on the ground after the unsummon", id)
		}
	}
	if atPet == len(dropped) {
		t.Fatal("every overflowed item landed on the pet's exact spot, want a random offset")
	}
}
