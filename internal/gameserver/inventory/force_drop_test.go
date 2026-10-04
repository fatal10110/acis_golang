package inventory

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// TestForceDropItemIgnoresDroppableFlag: a worn weapon its template marks
// non-droppable is refused by DropItem but dropped whole, coming off first,
// by ForceDropItem, as a cursed weapon leaves its dead holder.
func TestForceDropItemIgnoresDroppableFlag(t *testing.T) {
	const weaponID int32 = 8190
	tmpl := &item.Template{
		ID: weaponID, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1,
		Weapon: &item.WeaponDetail{Type: item.WeaponSword},
	}
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{tmpl}))
	weapon := inv.AddNew(weaponID, 1, 500)
	inv.EquipItem(weapon, tmpl)
	inv.DrainUpdates()
	svc := NewService(&testIDs{next: 900})

	if _, ok, err := svc.DropItem(inv, weapon.ObjectID, 1); ok || err != nil {
		t.Fatalf("DropItem = ok %v, err %v; want refused", ok, err)
	}
	res, ok, err := svc.ForceDropItem(inv, weapon.ObjectID)
	if err != nil || !ok {
		t.Fatalf("ForceDropItem = ok %v, err %v; want dropped", ok, err)
	}
	if res.Dropped == nil || res.Dropped.ObjectID != weapon.ObjectID || res.Dropped.Count != 1 || !res.EquipmentChanged {
		t.Fatalf("dropped = %+v, equipment changed %v; want the worn weapon itself", res.Dropped, res.EquipmentChanged)
	}
	if inv.ItemByObjectID(weapon.ObjectID) != nil {
		t.Fatal("weapon still in the inventory")
	}
	if _, ok, _ := svc.ForceDropItem(inv, weapon.ObjectID); ok {
		t.Fatal("ForceDropItem dropped an item no longer held")
	}
}
