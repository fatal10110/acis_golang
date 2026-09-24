package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestInventoryDeliverySkipsDetachedOrDespawnedOwners(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}}})
	updates := task.NewInventoryUpdates()
	live := &livePlayer{Character: &player.Character{ID: 1}}
	live.markDetaching()

	playerInv := itemcontainer.NewPlayerInventoryWithDelivery(1, templates, &playerInventoryDelivery{updates: updates, live: live, character: live.Character}, nil)
	playerInv.AddNew(1, 1, 1)
	if updates.Contains(playerInv) {
		t.Fatal("detached player inventory registered for delivery")
	}

	petInv := itemcontainer.NewPetInventoryWithDelivery(2, templates, &petInventoryDelivery{updates: updates, live: &livePlayer{Character: &player.Character{ID: 1}}, state: world.New()}, nil)
	petInv.AddNew(1, 1, 2)
	if updates.Contains(petInv) {
		t.Fatal("despawned pet inventory registered for delivery")
	}
}
