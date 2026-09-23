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
	if pet == nil || petInv == nil || ground == nil || ground.Template == nil || ground.Count() <= 0 {
		return PickupResult{}, PickupNoop
	}
	if !PickupAvailable(pet) {
		return PickupResult{}, PickupPetUnavailable
	}

	picked := ground.Instance.Clone()
	herb := ground.Template.EtcItem != nil && ground.Template.EtcItem.Type == modelitem.EtcItemHerb
	if !herb && ForbiddenForPet(picked, ground.Template) {
		return PickupResult{}, PickupItemNotForPets
	}
	// Every rejection from here on puts the claimed item back on the ground.
	if !ground.Claim() {
		return PickupResult{}, PickupTaken
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(picked, ground.Template)) {
		ground.Release()
		return PickupResult{}, PickupPetCannotCarryMore
	}
	if inventory.LootLocked(ground.Instance.OwnerID, pet.OwnerID()) {
		ground.Release()
		return PickupResult{}, PickupLootLocked
	}
	if herb {
		return PickupResult{Herb: picked}, PickupOK
	}

	result, absorbed := petInv.Add(picked)
	if result == nil {
		ground.Release()
		return PickupResult{}, PickupNoop
	}
	actions := []inventory.Persist{inventory.Save(result)}
	if absorbed {
		actions = []inventory.Persist{inventory.Update(result), inventory.Delete(ground.Instance.OwnerID, ground.ObjectID())}
	}
	return PickupResult{Persist: actions}, PickupOK
}
