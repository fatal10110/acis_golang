package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The owner approved diverging from the reference server here (#5088,
// #5089): an item taken back from a pet, or returned by a departing pet,
// goes through the owner's slot limit with a merging stack needing no slot,
// and a take-back also through its weight limit. The reference takes items
// back past both limits and drops a returning stack that would have merged.

const (
	shortSwordID = int32(30)   // unstackable fixture sword
	stemID       = int32(9701) // stackable, weighted, see bootTakeBackOwner
	animalBoneID = int32(9702) // stackable, weighted, see bootTakeBackOwner
	materialWt   = 2
)

// bootTakeBackOwner is bootOwnerWithCollar with two stackable materials
// of weight materialWt a pet may carry added to the fixture catalog.
func bootTakeBackOwner(t *testing.T, seeds ...seedItem) *petWorld {
	t.Helper()
	catalog := gameservertest.ItemTemplates().All()
	for _, m := range []struct {
		id   int32
		name string
	}{{stemID, "Stem"}, {animalBoneID, "Animal Bone"}} {
		catalog = append(catalog, &item.Template{
			ID: m.id, Name: m.name, Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Dropable: true, Tradable: true, Destroyable: true,
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemMaterial}, Weight: materialWt,
		})
	}
	return bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(item.NewTable(catalog))}, seeds...)
}

// requireSplit checks how many units of templateID the owner and its pet
// hold, in memory and in their saved rows.
func (h *petWorld) requireSplit(t *testing.T, wolf *summon.Actor, templateID int32, owner, pet int) {
	t.Helper()
	held := countTemplate(h.ownerInventory(t), templateID)
	carried := countTemplate(wolf.PetInventory(), templateID)
	if held != owner || carried != pet {
		t.Fatalf("item %d in memory = owner %d pet %d, want owner %d pet %d", templateID, held, carried, owner, pet)
	}
	if got := h.ownerItemCount(t, templateID); got != owner {
		t.Fatalf("owner rows of item %d = %d, want %d", templateID, got, owner)
	}
	if got := h.petRowCount(t, templateID); got != pet {
		t.Fatalf("pet rows of item %d = %d, want %d", templateID, got, pet)
	}
}

func countTemplate(inv *itemcontainer.Inventory, templateID int32) int {
	n := 0
	for _, inst := range inv.Items() {
		if inst.TemplateID == templateID {
			n += inst.Snapshot().Count
		}
	}
	return n
}

// carriedWeight sums the weight of everything inv holds.
func carriedWeight(t *testing.T, inv *itemcontainer.Inventory) int {
	t.Helper()
	w := 0
	for _, inst := range inv.Items() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			t.Fatalf("no template for item %d", inst.TemplateID)
		}
		w += int(tmpl.Weight) * inst.Snapshot().Count
	}
	return w
}

// requireRefusal checks that a take-back answered only with the system
// message msgID.
func requireRefusal(t *testing.T, frames [][]byte, msgID int) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("refused take-back frames = %x, want only system message %d", frameOpcodes(frames), msgID)
	}
	assertStaticSystemMessage(t, frames[0], msgID)
}

// requireTakenBack checks that a take-back moved the item: the owner's and
// the pet's inventory updates.
func requireTakenBack(t *testing.T, frames [][]byte) {
	t.Helper()
	if countOpcode(frames, serverpackets.OpcodeInventoryUpdate) != 1 || countOpcode(frames, serverpackets.OpcodePetInventoryUpdate) != 1 {
		t.Fatalf("take-back frames = %x, want the owner's and the pet's inventory updates", frameOpcodes(frames))
	}
}

// TestTakeBackRefusedWhenOwnerSlotsFull takes items back from the wolf to an
// owner with no free slot: a new stack and an unstackable item are refused
// with SLOTS_FULL and stay with the pet, while units of a stack the owner
// holds still merge into it.
func TestTakeBackRefusedWhenOwnerSlotsFull(t *testing.T) {
	t.Parallel()
	h := bootTakeBackOwner(t,
		seedItem{TemplateID: animalBoneID, Count: 10},
		seedItem{TemplateID: stemID, Count: 10},
		seedItem{TemplateID: shortSwordID, Count: 1},
	)
	wolf, _ := h.spawnWolf(t)
	stem, sword := h.seededItem(t, stemID), h.seededItem(t, shortSwordID)
	h.giveToPet(t, stem, 10)
	h.giveToPet(t, sword, 1)
	h.giveToPet(t, h.seededItem(t, animalBoneID), 4)
	// The owner keeps the collar and six bones, and no free slot.
	h.srv.SetInventorySlotLimit(t, h.ownerID, h.ownerInventory(t).Size())

	requireRefusal(t, h.sendAndTick(t, encodeRequestGetItemFromPet(stem, 10)), serverpackets.SystemMessageSlotsFull)
	requireRefusal(t, h.sendAndTick(t, encodeRequestGetItemFromPet(sword, 1)), serverpackets.SystemMessageSlotsFull)
	h.requireSplit(t, wolf, stemID, 0, 10)
	h.requireSplit(t, wolf, shortSwordID, 0, 1)
	h.requireSplit(t, wolf, animalBoneID, 6, 4)

	bone := wolf.PetInventory().ItemByTemplateID(animalBoneID).ObjectID
	requireTakenBack(t, h.sendAndTick(t, encodeRequestGetItemFromPet(bone, 4)))
	h.requireSplit(t, wolf, animalBoneID, 10, 0)
}

// TestTakeBackRefusedWhenOwnerOverweight takes stems back from the wolf to
// an owner with room for the weight of nine: all ten are refused with
// WEIGHT_LIMIT_EXCEEDED and stay with the pet, while nine still move.
func TestTakeBackRefusedWhenOwnerOverweight(t *testing.T) {
	t.Parallel()
	h := bootTakeBackOwner(t, seedItem{TemplateID: stemID, Count: 10})
	wolf, _ := h.spawnWolf(t)
	stem := h.seededItem(t, stemID)
	h.giveToPet(t, stem, 10)
	h.srv.SetInventoryWeightLimit(t, h.ownerID, carriedWeight(t, h.ownerInventory(t))+10*materialWt-1)

	requireRefusal(t, h.sendAndTick(t, encodeRequestGetItemFromPet(stem, 10)), serverpackets.SystemMessageWeightLimitExceeded)
	h.requireSplit(t, wolf, stemID, 0, 10)

	requireTakenBack(t, h.sendAndTick(t, encodeRequestGetItemFromPet(stem, 9)))
	h.requireSplit(t, wolf, stemID, 9, 1)
}
