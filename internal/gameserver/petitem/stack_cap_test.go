package petitem

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Owner-approved divergence (#5087): a stack move that would push the
// receiver's stack past MaxInt32 is refused before the source changes.
// These cover the rechecks made inside the item operation, after the
// precheck already passed.

// TestStoreGroundRefusesStackGrownPastCountLimitAfterClaim: the pet's
// stack grows between ClaimGround and StoreGround, so the store must
// refuse instead of clamping the ground count away.
func TestStoreGroundRefusesStackGrownPastCountLimitAfterClaim(t *testing.T) {
	templates := testTemplates()
	petInv := itemcontainer.NewPetInventory(2, templates)
	pet := mustTestPet(t, summon.PetConfig{ObjectID: 2, NPCID: 12077, Inventory: petInv})
	tmpl, ok := templates.Get(item.AdenaID)
	if !ok {
		t.Fatal("adena template missing")
	}
	ground, err := grounditem.New(item.Instance{ObjectID: 900, TemplateID: item.AdenaID, Count: 40, ManaLeft: -1}, tmpl)
	if err != nil {
		t.Fatalf("ground item: %v", err)
	}

	if herb, held, failure := ClaimGround(pet, petInv, ground); failure != PickupOK || herb || !held {
		t.Fatalf("ClaimGround = (%v, %v, %v), want held non-herb OK", herb, held, failure)
	}
	held := petInv.AddNew(item.AdenaID, math.MaxInt32-10, 500)
	petInv.DrainUpdates()

	persist, stored := StoreGround(petInv, ground)

	if stored || persist != nil {
		t.Fatalf("StoreGround = (%+v, %v), want refusal with no writes", persist, stored)
	}
	if got := held.Snapshot().Count; got != math.MaxInt32-10 {
		t.Fatalf("pet stack count = %d, want unchanged %d", got, math.MaxInt32-10)
	}
	if petInv.ItemByObjectID(ground.ObjectID()) != nil {
		t.Fatal("ground item entered the pet inventory despite refusal")
	}
	if ground.Count() != 40 {
		t.Fatalf("ground count = %d, want 40 left for release", ground.Count())
	}
}

// TestTransferRefusesWornStackPastCountLimitBeforeUnequip: moving the
// whole of a worn stackable item into a receiver stack with no headroom is
// refused, and the item stays worn in the source.
func TestTransferRefusesWornStackPastCountLimitBeforeUnequip(t *testing.T) {
	const bowID, arrowID int32 = 40, 41
	templates := item.NewTable([]*item.Template{
		{ID: bowID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponBow}},
		{ID: arrowID, Kind: item.KindEtcItem, Slot: item.SlotLHand, Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Duration: -1, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
	})
	from := itemcontainer.NewPlayerInventory(1, templates)
	to := itemcontainer.NewPetInventory(2, templates)
	bow := from.AddNew(bowID, 1, 500)
	arrows := from.AddNew(arrowID, 100, 501)
	bowTmpl, _ := templates.Get(bowID)
	arrowTmpl, _ := templates.Get(arrowID)
	from.EquipItem(bow, bowTmpl)
	from.EquipItem(arrows, arrowTmpl)
	if !arrows.Equipped() {
		t.Fatalf("setup: arrows = %+v, want worn", arrows.Snapshot())
	}
	held := to.AddNew(arrowID, math.MaxInt32-50, 600)
	from.DrainUpdates()
	to.DrainUpdates()

	res, moved, err := NewService(&testIDs{next: 900}).Transfer(from, to, arrows.ObjectID, 100)
	if err != nil {
		t.Fatalf("Transfer error = %v", err)
	}
	if moved || len(res.Persist) != 0 || len(res.Unequipped) != 0 {
		t.Fatalf("Transfer = (%+v, %v), want refusal with nothing unequipped or persisted", res, moved)
	}
	if !arrows.Equipped() || from.ItemByObjectID(arrows.ObjectID) != arrows || arrows.Snapshot().Count != 100 {
		t.Fatalf("source arrows = %+v, want still worn with 100", arrows.Snapshot())
	}
	if got := held.Snapshot().Count; got != math.MaxInt32-50 {
		t.Fatalf("receiver stack count = %d, want unchanged %d", got, math.MaxInt32-50)
	}
}
