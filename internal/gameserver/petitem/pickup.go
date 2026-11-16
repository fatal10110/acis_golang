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

// PickupAvailable reports whether pet can currently pick up ground items.
func PickupAvailable(pet *summon.Actor) bool {
	return pet != nil && !pet.Dead() && !pet.OutOfControl()
}

// ClaimGround runs every check a pet's pickup of ground must pass, once the
// pet stands within reach of it (a pet that cannot act is refused), and claims
// it; herb reports a herb, which the pet uses instead of carrying. held
// reports the caller holds the claim: on PickupOK it then stores the item
// (StoreGround) or hands it on, as the owner's party loot rule does; on a
// rejection it lets the item go.
func ClaimGround(pet *summon.Actor, petInv *itemcontainer.Inventory, ground *grounditem.Item) (herb, held bool, failure PickupFailure) {
	if pet == nil || petInv == nil || ground == nil || ground.Template == nil || ground.Count() <= 0 {
		return false, false, PickupNoop
	}
	if pet.DenyAIAction() {
		return false, false, PickupPetUnavailable
	}

	picked := ground.Instance.Clone()
	herb = ground.Template.EtcItem != nil && ground.Template.EtcItem.Type == modelitem.EtcItemHerb
	if Ammunition(ground.Template) {
		return false, false, PickupItemNotForPets
	}
	if !ground.Claim() {
		return false, false, PickupTaken
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(picked, ground.Template)) {
		return false, true, PickupPetCannotCarryMore
	}
	// A herb is used, never carried; anything else must fit the pet's
	// stack of it within the item count limit.
	if !herb && !stackFits(petInv, ground.Template.ID, ground.Template, ground.Count()) {
		return false, true, PickupPetCannotCarryMore
	}
	if ownerLootLocked(pet, ground.Instance.Snapshot().OwnerID) {
		return false, true, PickupLootLocked
	}
	return herb, true, PickupOK
}

// StoreGround moves a claimed ground item into the pet inventory and
// returns the writes that persist it. It stores nothing, reporting false,
// when the pet's stack of the item can no longer take the ground count
// without passing the item count limit.
func StoreGround(petInv *itemcontainer.Inventory, ground *grounditem.Item) ([]inventory.Persist, bool) {
	if !stackFits(petInv, ground.Template.ID, ground.Template, ground.Count()) {
		return nil, false
	}
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
