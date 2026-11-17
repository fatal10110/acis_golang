package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: UseItem.java:89-138. A pet equipment item (PET weapon or
// armor type) used from the owner's own inventory is refused, by its
// system message alone, when no pet is out (CANNOT_EQUIP_PET_ITEM), the pet
// cannot wear it (PET_CANNOT_USE_ITEM), the pet is dead
// (CANNOT_GIVE_ITEMS_TO_DEAD_PET), the pet has no free slot
// (YOUR_PET_CANNOT_CARRY_ANY_MORE_ITEMS) or no weight to spare
// (UNABLE_TO_PLACE_ITEM_YOUR_PET_IS_TOO_ENCUMBERED), in that order;
// otherwise one unit moves to the pet, which puts it on (PET_PUT_ON_S1).

// heavyPetWeaponID is a wolf weapon no pet can carry.
const heavyPetWeaponID = int32(9605)

// bootGearOwner boots the owner holding seeds, with a collar calling out a
// wolf of npcID, on the suite catalog plus a wearable and a too-heavy wolf
// weapon.
func bootGearOwner(t *testing.T, npcID int, seeds ...seedItem) *petWorld {
	t.Helper()
	wolf := wolfTemplate()
	wolf.ID = npcID
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wolfCollarID, NPCID: int32(npcID), SummonType: 1}})
	if err != nil {
		t.Fatal(err)
	}
	heavy := petWeaponTemplate()
	heavy.ID, heavy.Name, heavy.Weight = heavyPetWeaponID, "Heavy Wolf Weapon", 1_000_000
	return bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf})),
		gameservertest.WithSummonItems(summonItems),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), petWeaponTemplate(), heavy))),
	}, seeds...)
}

// requireOwnerUseRefused uses templateID from the owner's inventory and
// requires the refusal message alone, with the item still the owner's.
func (h *petWorld) requireOwnerUseRefused(t *testing.T, templateID int32, messageID int) {
	t.Helper()
	frames := h.sendAndTick(t, encodeUseItem(h.seededItem(t, templateID), false))
	if len(frames) != 1 {
		t.Fatalf("refused pet item use sent %x, want the refusal message alone", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], messageID)
	if got := h.ownerItemCount(t, templateID); got != 1 {
		t.Fatalf("owner holds %d of item %d after the refusal, want 1", got, templateID)
	}
	if got := h.petRowCount(t, templateID); got != 0 {
		t.Fatalf("pet holds %d of item %d after the refusal, want 0", got, templateID)
	}
}

// TestOwnerUsePetWeaponPutsItOnThePet uses a wolf weapon from the owner's
// inventory with the armed wolf out: it moves to the wolf's right hand,
// PET_PUT_ON_S1 names it, the owner's InventoryUpdate and the wolf's
// PetInventoryUpdate follow, and the row is saved as pet equipment.
func TestOwnerUsePetWeaponPutsItOnThePet(t *testing.T) {
	t.Parallel()
	h := bootGearOwner(t, armedWolfNPCID, seedItem{TemplateID: fixturePetWeaponID, Count: 1})
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	weaponID := h.seededItem(t, fixturePetWeaponID)

	frames := h.sendAndTick(t, encodeUseItem(weaponID, false))
	put := findSystemMessage(t, frames, serverpackets.SystemMessagePetPutOnS1)
	if put == nil {
		t.Fatalf("use frames = %x, want PET_PUT_ON_S1", frameOpcodes(frames))
	}
	assertSystemMessageItem(t, put, serverpackets.SystemMessagePetPutOnS1, fixturePetWeaponID)
	ops := frameOpcodes(frames)
	if !slices.Contains(ops, serverpackets.OpcodeInventoryUpdate) || !slices.Contains(ops, serverpackets.OpcodePetInventoryUpdate) {
		t.Fatalf("use frames = %x, want the owner's InventoryUpdate and the PetInventoryUpdate", ops)
	}
	if held := wolf.PetInventory().ItemAt(itemcontainer.RHand); held == nil || held.ObjectID != weaponID {
		t.Fatalf("wolf right hand = %+v, want weapon %d", held, weaponID)
	}
	row := mustPersistedItem(t, h.srv, h.ownerID, weaponID)
	if row.Location != item.LocationPetEquip || row.LocationData != itemcontainer.RHand {
		t.Fatalf("weapon row = %+v, want pet equipment in the right hand", row)
	}
	if got := h.ownerItemCount(t, fixturePetWeaponID); got != 0 {
		t.Fatalf("owner still holds %d wolf weapons", got)
	}
}

// TestOwnerUsePetWeaponWithoutPet: with no pet out the use reads
// CANNOT_EQUIP_PET_ITEM.
func TestOwnerUsePetWeaponWithoutPet(t *testing.T) {
	t.Parallel()
	h := bootGearOwner(t, armedWolfNPCID, seedItem{TemplateID: fixturePetWeaponID, Count: 1})
	h.settleInventoryUpdates(t)
	h.requireOwnerUseRefused(t, fixturePetWeaponID, serverpackets.SystemMessageCannotEquipPetItem)
}

// TestOwnerUsePetWeaponPetCannotWear: a pet that cannot wear wolf gear
// reads PET_CANNOT_USE_ITEM.
func TestOwnerUsePetWeaponPetCannotWear(t *testing.T) {
	t.Parallel()
	h := bootGearOwner(t, wolfNPCID, seedItem{TemplateID: fixturePetWeaponID, Count: 1})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	h.requireOwnerUseRefused(t, fixturePetWeaponID, serverpackets.SystemMessagePetCannotUseItem)
}

// TestOwnerUsePetWeaponDeadPet: a dead pet that could wear the weapon
// reads CANNOT_GIVE_ITEMS_TO_DEAD_PET.
func TestOwnerUsePetWeaponDeadPet(t *testing.T) {
	t.Parallel()
	h := bootGearOwner(t, armedWolfNPCID, seedItem{TemplateID: fixturePetWeaponID, Count: 1})
	wolf, _ := h.spawnWolf(t)
	killPet(t, h, wolf)
	h.settleInventoryUpdates(t)
	h.requireOwnerUseRefused(t, fixturePetWeaponID, serverpackets.SystemMessageCannotGiveItemsToDeadPet)
}

// TestOwnerUsePetWeaponPetSlotsFull: with every one of the wolf's 12 slots
// taken the use reads YOUR_PET_CANNOT_CARRY_ANY_MORE_ITEMS.
func TestOwnerUsePetWeaponPetSlotsFull(t *testing.T) {
	t.Parallel()
	h := bootGearOwner(t, armedWolfNPCID, seedItem{TemplateID: fixturePetWeaponID, Count: 1})
	h.spawnWolf(t)
	for range 12 {
		h.srv.GivePetItem(t, h.ownerID, 30, 1)
	}
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)
	h.requireOwnerUseRefused(t, fixturePetWeaponID, serverpackets.SystemMessagePetCannotCarryMoreItems)
}

// TestOwnerUsePetWeaponTooHeavy: a weapon heavier than the wolf can carry
// reads UNABLE_TO_PLACE_ITEM_YOUR_PET_IS_TOO_ENCUMBERED.
func TestOwnerUsePetWeaponTooHeavy(t *testing.T) {
	t.Parallel()
	h := bootGearOwner(t, armedWolfNPCID, seedItem{TemplateID: heavyPetWeaponID, Count: 1})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	h.requireOwnerUseRefused(t, heavyPetWeaponID, serverpackets.SystemMessagePetTooEncumbered)
}
