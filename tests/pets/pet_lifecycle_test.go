package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestGiveItemToPetMovesStackAndPersists drives the give flow end to end:
// part of an owner stack moves into the pet's carried inventory, both sides
// get their update frames in the transfer's registration order, and the
// items rows reflect owner and pet stacks.
func TestGiveItemToPetMovesStackAndPersists(t *testing.T) {
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100})
	h.spawnWolf(t)
	adenaID := h.seededItem(t, item.AdenaID)

	h.giveToPet(t, adenaID, 30)

	h.srv.FlushItems(t)
	petObj, ok := h.srv.State.Summon(h.ownerID)
	if !ok {
		t.Fatal("active pet missing after give")
	}
	rows, err := h.srv.Items.ListByOwner(petCtx(), petObj.ObjectID())
	if err != nil {
		t.Fatalf("list pet items: %v", err)
	}
	var petStack int
	for _, row := range rows {
		if row.TemplateID == item.AdenaID {
			if row.Location != item.LocationPet || row.OwnerID != petObj.ObjectID() {
				t.Fatalf("pet stack = %+v, want LocationPet owned by the pet", row)
			}
			petStack += row.Count
		}
	}
	if petStack != 30 {
		t.Fatalf("pet adena rows total %d, want 30", petStack)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 70 {
		t.Fatalf("owner adena count = %d, want 70", got)
	}
}

// petInventoryAdena returns the adena stack snapshot currently carried by
// the active pet.
func (h *petWorld) petInventoryAdena(t *testing.T) item.InstanceState {
	t.Helper()
	obj, ok := h.srv.State.Summon(h.ownerID)
	if !ok {
		t.Fatal("active pet missing")
	}
	inst := obj.(*summon.Actor).PetInventory().ItemByTemplateID(item.AdenaID)
	if inst == nil {
		t.Fatal("pet carries no adena")
	}
	return inst.Snapshot()
}

// TestGetItemFromPetReturnsStackToOwner pulls a carried stack back out.
func TestGetItemFromPetReturnsStackToOwner(t *testing.T) {
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100})
	h.spawnWolf(t)
	adenaID := h.seededItem(t, item.AdenaID)
	h.giveToPet(t, adenaID, 30)

	petStackObj := h.petInventoryAdena(t).ObjectID
	h.client.Send(encodeRequestGetItemFromPet(petStackObj, 15))
	testsupport.SyncBarrier(t, h.client, func() {
		h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
	drainFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)
	if len(frames) < 2 {
		t.Fatalf("take-back frames = %d, want PetInventoryUpdate then InventoryUpdate", len(frames))
	}
	assertFrameOpcode(t, frames[0], serverpackets.OpcodePetInventoryUpdate, "pet-side update")
	assertFrameOpcode(t, frames[1], serverpackets.OpcodeInventoryUpdate, "owner-side update")

	if got := h.ownerItemCount(t, item.AdenaID); got != 85 {
		t.Fatalf("owner adena count = %d, want 85", got)
	}
}

// TestPetPickupGroundItem commands the pet to loot a dropped stack: GetItem
// names the pet as picker, the ground object despawns, and the stack lands
// in the pet's carried inventory.
func TestPetPickupGroundItem(t *testing.T) {
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 40})
	petActor, _ := h.spawnWolf(t)
	adenaID := h.seededItem(t, item.AdenaID)

	h.client.Send(encodeRequestDropItem(adenaID, 40, 10, 20, 30))
	frame := mustRead(t, h.client, "DropItem broadcast")
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	groundID := r.ReadInt32()

	h.srv.InventoryUpdates.Tick()
	drainFrames(t, h.client)

	h.client.Send(encodeRequestPetGetItem(groundID))
	frame = mustRead(t, h.client, "pickup GetItem")
	assertFrameOpcode(t, frame, serverpackets.OpcodeGetItem, "GetItem")
	r = wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != petActor.ObjectID() {
		t.Fatalf("GetItem picker id = %d, want pet id %d", got, petActor.ObjectID())
	}
	if got := r.ReadInt32(); got != groundID {
		t.Fatalf("GetItem ground id = %d, want %d", got, groundID)
	}
	assertFrameOpcode(t, mustRead(t, h.client, "DeleteObject"), serverpackets.OpcodeDeleteObject, "DeleteObject")

	h.srv.InventoryUpdates.Tick()
	assertFrameOpcode(t, mustRead(t, h.client, "PetInventoryUpdate"), serverpackets.OpcodePetInventoryUpdate, "PetInventoryUpdate")

	if _, ok := h.srv.State.Object(groundID); ok {
		t.Fatal("ground object still present after pickup")
	}
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), petActor.ObjectID())
	if err != nil {
		t.Fatalf("list pet items: %v", err)
	}
	picked := 0
	for _, row := range rows {
		if row.ObjectID == groundID {
			picked = row.Count
			if row.Location != item.LocationPet || row.OwnerID != petActor.ObjectID() {
				t.Fatalf("picked row = %+v, want LocationPet owned by the pet", row)
			}
		}
	}
	if picked != 40 {
		t.Fatalf("picked adena count = %d, want 40", picked)
	}
}

// TestFeedPetConsumesFoodAndRaisesMealGauge feeds wolf food through the
// pet-inventory use flow: the feed skill fires as the pet's own visual, one
// unit is consumed, and the meal gauge stays capped (the pet spawns fed to
// its max meal) through the return-time save.
func TestFeedPetConsumesFoodAndRaisesMealGauge(t *testing.T) {
	h := bootOwnerWithCollar(t, seedItem{TemplateID: wolfFoodID, Count: 2})
	petActor, _ := h.spawnWolf(t)
	foodID := h.seededItem(t, wolfFoodID)

	h.giveToPet(t, foodID, 2)

	h.client.Send(encodeRequestPetUseItem(foodID))
	frame := mustRead(t, h.client, "feed MagicSkillUse")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "feed MagicSkillUse")
	r := wire.NewReader(frame[1:])
	caster, _, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != petActor.ObjectID() || skill != wolfFeedSkill {
		t.Fatalf("feed cast = caster %d skill %d, want pet %d/%d", caster, skill, petActor.ObjectID(), wolfFeedSkill)
	}

	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)
	var fed bool
	for _, f := range frames {
		if f[0] == serverpackets.OpcodePetInventoryUpdate {
			fed = true
		}
	}
	if !fed {
		t.Fatalf("frames after feeding = opcodes %x, want a PetInventoryUpdate for the consumed unit", frameOpcodes(frames))
	}

	h.returnPet(t)
	state := h.savedPetState(t)
	if state.Fed < wolfMaxMeal-wolfFeedAmount || state.Fed > wolfMaxMeal {
		t.Fatalf("saved meal gauge = %d, want capped at %d after feeding from full", state.Fed, wolfMaxMeal)
	}
	if got := h.ownerItemCount(t, wolfFoodID); got != 1 {
		t.Fatalf("owner food count after return = %d, want 1 (the fed unit consumed, the remainder came back)", got)
	}
}

// frameOpcodes renders drained frames as an opcode string for failure text.
func frameOpcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}
