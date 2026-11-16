package petitem

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// TestClaimGroundRejectionKeepsTheClaim: a rejection after the claim leaves
// the caller holding it, so the caller's let-go decides whether the item
// goes back on the ground; one before the claim leaves the item unclaimed.
func TestClaimGroundRejectionKeepsTheClaim(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	petInv.SlotLimit = 1
	petInv.AddNew(20, 1, 600)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv, Owner: &pickupTestOwner{id: 1}})
	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	if _, held, failure := ClaimGround(pet, petInv, ground); failure != PickupPetCannotCarryMore || !held {
		t.Fatalf("ClaimGround = (held %v, %v), want held PickupPetCannotCarryMore", held, failure)
	}
	if ground.Claim() {
		t.Fatal("the rejected pickup let go of the claim itself")
	}

	if _, held, failure := ClaimGround(pet, petInv, ground); failure != PickupTaken || held {
		t.Fatalf("ClaimGround on a held item = (held %v, %v), want unheld PickupTaken", held, failure)
	}
}
