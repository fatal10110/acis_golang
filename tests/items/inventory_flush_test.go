package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDestroyFlushesBatchedUpdate drives a destroy, which has no synchronous
// reply of its own: an ItemList sync barrier proves the server processed it,
// one InventoryUpdates tick delivers exactly one modified-entry
// InventoryUpdate, and the items row matches the remaining stack.
func TestDestroyFlushesBatchedUpdate(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(potion, 2))
	// The background batching tick may deliver the weight refresh and the
	// update ahead of the barrier reply; wait for the ItemList itself.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame != nil && frame[0] == serverpackets.OpcodeItemList {
			break
		}
	}
	srv.InventoryUpdates.Tick()
	e := readInventoryUpdateFor(t, c, potion, 3)
	if e.state != 2 {
		t.Fatalf("InventoryUpdate state = %d, want modified (2)", e.state)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 3 {
		t.Fatalf("persisted potion count = %d, want 3", inst.Count)
	}
}

// TestCrystallizeGrantsCrystals crystallizes the D-grade sword and requires
// the crystallized message plus one batched InventoryUpdate carrying both
// the removed source row and the added crystal reward, mirrored by the items
// rows.
func TestCrystallizeGrantsCrystals(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
		t.Fatalf("grant crystallize skill: %v", err)
	}
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeRequestCrystallizeItem(weapon, 1))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "crystallized SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageItemCrystallized {
		t.Fatalf("message id = %d, want ItemCrystallized (%d)", id, serverpackets.SystemMessageItemCrystallized)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("message params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
		t.Fatalf("param type = %d, want item name", typ)
	}
	if itemID := r.ReadInt32(); itemID != 30 {
		t.Fatalf("param item id = %d, want 30", itemID)
	}

	srv.InventoryUpdates.Tick()
	entries := readInventoryUpdateEntries(t, c.Read())
	var removedWeapon, addedCrystal bool
	for _, e := range entries {
		if e.state == 3 && e.objID == weapon && e.itemID == 30 {
			removedWeapon = true
		}
		if e.state == 1 && e.itemID == item.CrystalD.ItemID() && e.count == 10 {
			addedCrystal = true
		}
	}
	if !removedWeapon || !addedCrystal {
		t.Fatalf("crystallize InventoryUpdate entries = %+v, want removed weapon plus added 10 D crystals", entries)
	}

	srv.FlushItems(t)
	assertItemGone(t, srv, objID, weapon)
	crystalCount := 0
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == item.CrystalD.ItemID() {
			crystalCount += inst.Count
		}
	}
	if crystalCount != 10 {
		t.Fatalf("persisted crystal count after crystallize = %d, want 10", crystalCount)
	}
}

// TestCrystallizeWithoutSkillIsRejected pins the skill gate: without the
// crystallize skill the request answers CrystallizeLevelTooLow only, and the
// weapon row survives.
func TestCrystallizeWithoutSkillIsRejected(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeRequestCrystallizeItem(weapon, 1))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCrystallizeLevelTooLow)
	barrier(t, c)

	if inst := mustFindItem(t, srv, objID, weapon); inst.Count != 1 {
		t.Fatalf("weapon count after rejected crystallize = %d, want 1", inst.Count)
	}
}
