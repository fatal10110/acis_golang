package petitem

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
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
	// PickupPetTooEncumbered means the pet lacks remaining weight capacity.
	PickupPetTooEncumbered
)

// PickupResult carries item-store operations for a pet ground-item pickup.
type PickupResult struct {
	Persist []inventory.Persist
}

// PickupAvailable reports whether pet can currently pick up ground items.
func PickupAvailable(pet *summon.Actor) bool {
	return pet != nil && !pet.Dead() && !pet.OutOfControl()
}

// PickupGround validates and moves a ground item into a pet inventory.
func PickupGround(pet *summon.Actor, petInv *itemcontainer.Inventory, ground *grounditem.Item) (PickupResult, PickupFailure) {
	if pet == nil || petInv == nil || ground == nil || ground.Template == nil || ground.Count() <= 0 {
		return PickupResult{}, PickupNoop
	}
	if !PickupAvailable(pet) {
		return PickupResult{}, PickupPetUnavailable
	}

	picked := ground.Instance
	if ForbiddenForPet(&picked, ground.Template) {
		return PickupResult{}, PickupItemNotForPets
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(&picked, ground.Template)) {
		return PickupResult{}, PickupPetCannotCarryMore
	}
	if !petInv.ValidateWeight(int(ground.Template.Weight) * picked.Count) {
		return PickupResult{}, PickupPetTooEncumbered
	}

	result, absorbed := petInv.Add(&picked)
	if result == nil {
		return PickupResult{}, PickupNoop
	}
	actions := []inventory.Persist{inventory.Save(result)}
	if absorbed {
		actions = []inventory.Persist{inventory.Update(result), inventory.Delete(ground.ObjectID())}
	}
	return PickupResult{Persist: actions}, PickupOK
}
