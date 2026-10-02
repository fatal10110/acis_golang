package petitem

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// PickupFailure is the non-mutating reason a pet ground-item pickup failed.
type PickupFailure uint8

const (
	// PickupOK means the ground item was moved into the pet inventory.
	PickupOK PickupFailure = iota
	// PickupNoop means the request is invalid and should be ignored.
	PickupNoop
	// PickupPetUnavailable means the pet cannot pick up items right now.
	PickupPetUnavailable
	// PickupItemNotForPets means the item is forbidden for pets.
	PickupItemNotForPets
	// PickupPetCannotCarryMore means the pet lacks free inventory slots.
	PickupPetCannotCarryMore
	// PickupLootLocked means ground is owned by someone other than pet's owner.
	PickupLootLocked
	// PickupTaken means another pickup or the ground cleanup claimed ground
	// first, so there is nothing left to take.
	PickupTaken
)

// PickupResult carries item-store operations for a pet ground-item pickup.
type PickupResult struct {
	Persist []inventory.Persist
	Herb    *modelitem.Instance
}

// PickupAvailable reports whether pet can currently pick up ground items.
func PickupAvailable(pet *summon.Actor) bool {
	return pet != nil && !pet.Dead() && !pet.OutOfControl()
}

// PickupGround validates and moves a ground item into a pet inventory. It
// claims ground first (see grounditem.Item.Claim) and keeps the claim only on
// PickupOK, so the caller despawns an item no one else can take any more.
func PickupGround(pet *summon.Actor, petInv *itemcontainer.Inventory, ground *grounditem.Item) (PickupResult, PickupFailure) {
	herb, failure := ClaimGround(pet, petInv, ground)
	if failure != PickupOK {
		return PickupResult{}, failure
	}
	if herb {
		return PickupResult{Herb: ground.Instance.Clone()}, PickupOK
	}
	persist, ok := StoreGround(petInv, ground)
	if !ok {
		ground.Release()
		return PickupResult{}, PickupNoop
	}
	return PickupResult{Persist: persist}, PickupOK
}

// ClaimGround runs every check a pet's pickup of ground must pass and claims
// it, keeping the claim only on PickupOK; herb reports a herb, which the pet
// uses instead of carrying. The caller then stores the item (StoreGround) or
// hands it on, as the owner's party loot rule does.
func ClaimGround(pet *summon.Actor, petInv *itemcontainer.Inventory, ground *grounditem.Item) (herb bool, failure PickupFailure) {
	if pet == nil || petInv == nil || ground == nil || ground.Template == nil || ground.Count() <= 0 {
		return false, PickupNoop
	}
	if !PickupAvailable(pet) {
		return false, PickupPetUnavailable
	}

	picked := ground.Instance.Clone()
	herb = ground.Template.EtcItem != nil && ground.Template.EtcItem.Type == modelitem.EtcItemHerb
	if !herb && ForbiddenForPet(picked, ground.Template) {
		return false, PickupItemNotForPets
	}
	// Every rejection from here on puts the claimed item back on the ground.
	if !ground.Claim() {
		return false, PickupTaken
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(picked, ground.Template)) {
		ground.Release()
		return false, PickupPetCannotCarryMore
	}
	if ownerLootLocked(pet, ground.Instance.Snapshot().OwnerID) {
		ground.Release()
		return false, PickupLootLocked
	}
	return herb, PickupOK
}

// StoreGround moves a claimed ground item into the pet inventory and
// returns the writes that persist it.
func StoreGround(petInv *itemcontainer.Inventory, ground *grounditem.Item) ([]inventory.Persist, bool) {
	picked := ground.Instance.Clone()
	result, absorbed := petInv.Add(picked)
	if result == nil {
		return nil, false
	}
	if absorbed {
		return []inventory.Persist{inventory.Update(result), inventory.Delete(ground.Instance.OwnerID, ground.ObjectID())}, true
	}
	return []inventory.Persist{inventory.Save(result)}, true
}

// ownerLootLocked reports whether a ground item reserved to ownerID is
// locked against the pet's owner: the owner's party and command channel
// count as the owner. An owner that cannot tell its party is compared by id.
func ownerLootLocked(pet *summon.Actor, ownerID int32) bool {
	if owner, ok := pet.Owner(); ok {
		if picker, ok := owner.(inventory.LootPicker); ok {
			return inventory.LootLocked(ownerID, picker)
		}
	}
	return ownerID != 0 && ownerID != pet.OwnerID()
}
