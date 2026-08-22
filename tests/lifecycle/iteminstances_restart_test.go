package lifecycle

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// shadowSwordID is the shared catalog's time-limited shadow weapon
// (Duration 5 → 300 seconds of initial mana).
const shadowSwordID = 7884

// giveShadowSword inserts an equipped-ready shadow weapon whose row starts
// at the template's full mana, mirroring what a reward path persists.
func giveShadowSword(t *testing.T, srv *gameservertest.Server, ownerID int32) int32 {
	t.Helper()
	objectID := srv.NewObjectID()
	inst := item.Instance{
		ObjectID:   objectID,
		TemplateID: shadowSwordID,
		OwnerID:    ownerID,
		Count:      1,
		Location:   item.LocationInventory,
		ManaLeft:   300,
	}
	if err := srv.Items.Create(context.Background(), ownerID, inst); err != nil {
		t.Fatalf("seed shadow sword: %v", err)
	}
	return objectID
}

// TestItemInstanceDecaySurvivesRestart equips a shadow weapon, lets the
// server-side mana decay run three ticks with no client request involved,
// and shuts down. The next boot must carry the decayed mana across: the
// weapon comes back equipped, its re-equip pays the repeated-equip mana
// penalty, and the client sees the resulting remainder in the unequip
// InventoryUpdate while the row keeps the exact seconds.
func TestItemInstanceDecaySurvivesRestart(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := giveShadowSword(t, srv, objID)
	startInWorld(t, c)

	c.Send(encodeUseItem(sword, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	drainUntilQuiet(t, c)

	// Three server-side ticks: no packet is decoded, nothing calls a store.
	for i := 0; i < 3; i++ {
		srv.ShadowItems.Tick()
	}

	srv.Shutdown(t)

	var persisted *item.Instance
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == sword {
			persisted = inst
		}
	}
	if persisted == nil {
		t.Fatal("shadow sword row missing after shutdown")
	}
	if persisted.ManaLeft != 297 {
		t.Fatalf("persisted mana after shutdown = %d, want 297 (300 minus three decay ticks)", persisted.ManaLeft)
	}

	srv2 := gameservertest.Boot(t, gameservertest.WithWantChars(1))
	c2 := srv2.Client
	startInWorld(t, c2)
	// The restored weapon equips on world entry; re-tracking an already
	// decayed item costs the extra equip penalty minute (297 - 60 = 237).

	c2.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c2, "unequip UserInfo"), serverpackets.OpcodeUserInfo, "unequip UserInfo")
	assertFrameOpcode(t, readSkippingEquipNoise(t, c2, "unequip SystemMessage"), serverpackets.OpcodeSystemMessage, "unequip SystemMessage")

	srv2.InventoryUpdates.Tick()
	e := readInventoryUpdateFor(t, c2, sword, 1)
	if e.equipped != 0 {
		t.Fatalf("unequip InventoryUpdate equipped flag = %d, want 0", e.equipped)
	}
	if e.mana != 3 {
		t.Fatalf("unequip InventoryUpdate displayed mana = %d minutes, want 3 (237 seconds)", e.mana)
	}

	srv2.FlushItems(t)
	var restored *item.Instance
	for _, inst := range persistedItems(t, srv2, objID) {
		if inst.ObjectID == sword {
			restored = inst
		}
	}
	if restored == nil {
		t.Fatal("shadow sword row missing after second boot")
	}
	if restored.ManaLeft != 237 {
		t.Fatalf("persisted mana after restart = %d, want 237", restored.ManaLeft)
	}
}
