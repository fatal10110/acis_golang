package petitem

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// pickupResult is what a pet's own pickup of a ground item did: the writes
// that persist the stored item, or the herb it uses instead.
type pickupResult struct {
	Persist []inventory.Persist
	Herb    *modelitem.Instance
}

// claimAndStore runs a pet's pickup the way the server does when the pet
// keeps the item: ClaimGround, then StoreGround unless it is a herb.
func claimAndStore(pet *summon.Actor, petInv *itemcontainer.Inventory, ground *grounditem.Item) (pickupResult, PickupFailure) {
	herb, failure := ClaimGround(pet, petInv, ground)
	if failure != PickupOK {
		return pickupResult{}, failure
	}
	if herb {
		return pickupResult{Herb: ground.Instance.Clone()}, PickupOK
	}
	persist, ok := StoreGround(petInv, ground)
	if !ok {
		ground.Release()
		return pickupResult{}, PickupNoop
	}
	return pickupResult{Persist: persist}, PickupOK
}
