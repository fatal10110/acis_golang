package pets

import (
	"context"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The owner approved diverging from the reference server here (#5087): a
// move between owner and pet whose merged receiving stack would pass
// MaxInt32 is refused before the source is reduced, so no unit is destroyed
// by the count clamp. A full refusal mirrors the trade count-overflow fix.

// seedPetStack saves count of templateID as an item the pet carries, under
// its collar, before the pet is summoned, and returns its object id.
func (h *petWorld) seedPetStack(t *testing.T, templateID, count int32) int32 {
	t.Helper()
	stack := item.Instance{ObjectID: h.srv.NewObjectID(), TemplateID: templateID, OwnerID: h.collarID, Count: int(count), Location: item.LocationPet}
	if err := h.srv.Items.Create(context.Background(), h.collarID, stack); err != nil {
		t.Fatalf("seed pet stack: %v", err)
	}
	return stack.ObjectID
}

// requireAdena checks the owner's and the pet's adena, in memory and in
// their saved rows.
func (h *petWorld) requireAdena(t *testing.T, wolf *summon.Actor, owner, pet int) {
	t.Helper()
	held := 0
	if inst := h.ownerInventory(t).ItemByTemplateID(item.AdenaID); inst != nil {
		held = inst.Snapshot().Count
	}
	carried := 0
	if inst := wolf.PetInventory().ItemByTemplateID(item.AdenaID); inst != nil {
		carried = inst.Snapshot().Count
	}
	if held != owner || carried != pet {
		t.Fatalf("adena in memory = owner %d pet %d, want owner %d pet %d", held, carried, owner, pet)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != owner {
		t.Fatalf("owner adena rows = %d, want %d", got, owner)
	}
	if got := h.collarItemCount(t, item.AdenaID); got != pet {
		t.Fatalf("pet adena rows = %d, want %d", got, pet)
	}
}

// TestGiveToPetRefusesStackPastCountLimit gives 50 adena to a wolf carrying
// MaxInt32-10: the pet refuses it as a full pet does and the owner keeps all
// 50. The 10 that still fit then move, filling the pet's stack exactly.
func TestGiveToPetRefusesStackPastCountLimit(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 50})
	h.seedPetStack(t, item.AdenaID, math.MaxInt32-10)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	adena := h.seededItem(t, item.AdenaID)

	frames := h.sendAndTick(t, encodeRequestGiveItemToPet(adena, 50))
	if len(frames) != 1 {
		t.Fatalf("refused give frames = %x, want only PET_CANNOT_CARRY_MORE_ITEMS", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessagePetCannotCarryMoreItems)
	h.requireAdena(t, wolf, 50, math.MaxInt32-10)

	h.giveToPet(t, adena, 10)
	h.requireAdena(t, wolf, 40, math.MaxInt32)
}

// TestGetItemFromPetRefusesStackPastCountLimit takes 50 adena back from the
// wolf to an owner holding MaxInt32-10: the drag is refused with
// ActionFailed and the pet keeps all 50. The 10 that still fit then move.
func TestGetItemFromPetRefusesStackPastCountLimit(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: math.MaxInt32 - 10})
	carried := h.seedPetStack(t, item.AdenaID, 50)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)

	frames := h.sendAndTick(t, encodeRequestGetItemFromPet(carried, 50))
	if len(frames) != 1 {
		t.Fatalf("refused take-back frames = %x, want only ActionFailed", frameOpcodes(frames))
	}
	assertFrameOpcode(t, frames[0], serverpackets.OpcodeActionFailed, "ActionFailed")
	h.requireAdena(t, wolf, math.MaxInt32-10, 50)

	frames = h.sendAndTick(t, encodeRequestGetItemFromPet(carried, 10))
	if countOpcode(frames, serverpackets.OpcodeInventoryUpdate) != 1 || countOpcode(frames, serverpackets.OpcodePetInventoryUpdate) != 1 {
		t.Fatalf("take-back frames = %x, want the owner's and the pet's inventory updates", frameOpcodes(frames))
	}
	h.requireAdena(t, wolf, math.MaxInt32, 40)
}

// TestPetPickupRefusesStackPastCountLimit commands a wolf carrying
// MaxInt32-10 adena to loot 50 adena: it refuses as a full pet does and the
// adena stays on the ground. Once the owner takes 100 back, the same pile is
// picked up whole.
func TestPetPickupRefusesStackPastCountLimit(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	carried := h.seedPetStack(t, item.AdenaID, math.MaxInt32-10)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	x, y, z := wolf.Position()
	pile := h.seedGroundAt(t, item.AdenaID, 50, x+10, y, z)

	frames := h.petPickup(t, pile)
	if len(frames) != 1 {
		t.Fatalf("refused pickup frames = %x, want only PET_CANNOT_CARRY_MORE_ITEMS", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessagePetCannotCarryMoreItems)
	if _, ok := h.srv.State.Object(pile); !ok {
		t.Fatal("refused adena left the ground")
	}
	h.requireAdena(t, wolf, 0, math.MaxInt32-10)

	h.sendAndTick(t, encodeRequestGetItemFromPet(carried, 100))
	h.requireAdena(t, wolf, 100, math.MaxInt32-110)
	requirePickupHead(t, h.petPickup(t, pile), wolf, pile)
	if _, ok := h.srv.State.Object(pile); ok {
		t.Fatal("picked-up adena still on the ground")
	}
	h.requireAdena(t, wolf, 100, math.MaxInt32-60)
}
