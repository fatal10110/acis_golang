package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
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
	requireInventoryUpdateOrder(t, drainFrames(t, h.client), "take from pet",
		serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodeInventoryUpdate)

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

// TestPetPickupAttentionAnnouncedToObservers pins SummonAI.java:214-222:
// a pet looting armor or a weapon announces 1535 to nearby other clients
// with the owner name; the owner never receives the attention packet.
func TestPetPickupAttentionAnnouncedToObservers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		templateID int32
	}{
		{name: "weapon", templateID: 30},
		{name: "armor", templateID: 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := bootOwnerWithCollar(t, seedItem{TemplateID: tc.templateID, Count: 1})
			h.spawnWolf(t)
			itemID := h.seededItem(t, tc.templateID)

			h.srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
			observer := h.srv.DialClient(t, "player2", 1)
			startInWorld(t, observer)
			drainUntilQuiet(t, observer)
			drainUntilQuiet(t, h.client)

			h.client.Send(encodeRequestDropItem(itemID, 1, 10, 20, 30))
			frame := mustRead(t, h.client, "DropItem")
			assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
			r := wire.NewReader(frame[1:])
			r.ReadInt32()
			groundID := r.ReadInt32()
			drainUntilQuiet(t, observer)
			drainUntilQuiet(t, h.client)

			h.client.Send(encodeRequestPetGetItem(groundID))
			assertFrameOpcode(t, mustRead(t, h.client, "GetItem"), serverpackets.OpcodeGetItem, "GetItem")
			ownerFrames := drainFrames(t, h.client)
			observerFrames := drainFrames(t, observer)

			msg := findSystemMessage(t, observerFrames, serverpackets.SystemMessageAttentionS1PetPickedUpS2)
			if msg == nil {
				t.Fatalf("%s pet pickup produced no attention 1535 for the observer", tc.name)
			}
			assertPetPickupPlainParams(t, msg, "Owner", tc.templateID)
			if got := findSystemMessage(t, ownerFrames, serverpackets.SystemMessageAttentionS1PetPickedUpS2); got != nil {
				t.Fatal("owner received pet pickup attention message")
			}
			if got := findSystemMessage(t, ownerFrames, serverpackets.SystemMessageAttentionS1PetPickedUpS2S3); got != nil {
				t.Fatal("owner received enchanted pet pickup attention message")
			}
		})
	}
}

func TestPetPickupAttentionEnchantedWeapon(t *testing.T) {
	srv := bootPets(t)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	weaponID := srv.GiveItem(t, ownerID, 30, 1)
	inst := mustPersistedItem(t, srv, ownerID, weaponID)
	inst.EnchantLevel = 7
	if err := srv.Items.Update(petCtx(), inst); err != nil {
		t.Fatalf("seed enchant level: %v", err)
	}
	h := &petWorld{
		srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID,
		seeded: map[int32][]int32{30: {weaponID}},
	}
	startInWorld(t, h.client)
	h.spawnWolf(t)

	h.srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestDropItem(weaponID, 1, 10, 20, 30))
	frame := mustRead(t, h.client, "DropItem")
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	groundID := r.ReadInt32()
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestPetGetItem(groundID))
	assertFrameOpcode(t, mustRead(t, h.client, "GetItem"), serverpackets.OpcodeGetItem, "GetItem")
	observerFrames := drainFrames(t, observer)

	msg := findSystemMessage(t, observerFrames, serverpackets.SystemMessageAttentionS1PetPickedUpS2S3)
	if msg == nil {
		t.Fatal("enchanted weapon pet pickup produced no attention 1536 for the observer")
	}
	assertPetPickupEnchantParams(t, msg, "Owner", 7, 30)
}

func TestPetPickupAttentionSkippedForEtcItems(t *testing.T) {
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 40})
	h.spawnWolf(t)
	adenaID := h.seededItem(t, item.AdenaID)

	h.srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestDropItem(adenaID, 40, 10, 20, 30))
	frame := mustRead(t, h.client, "DropItem")
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	groundID := r.ReadInt32()
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestPetGetItem(groundID))
	assertFrameOpcode(t, mustRead(t, h.client, "GetItem"), serverpackets.OpcodeGetItem, "GetItem")
	for _, f := range drainFrames(t, observer) {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		id := systemMessageID(t, f)
		if id == serverpackets.SystemMessageAttentionS1PetPickedUpS2 || id == serverpackets.SystemMessageAttentionS1PetPickedUpS2S3 {
			t.Fatalf("etc pet pickup broadcast attention SystemMessage %d to the observer", id)
		}
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

func systemMessageID(t *testing.T, frame []byte) int {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	return int(wire.NewReader(frame[1:]).ReadInt32())
}

func findSystemMessage(t *testing.T, frames [][]byte, messageID int) []byte {
	t.Helper()
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == messageID {
			return f
		}
	}
	return nil
}

func mustPersistedItem(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) *item.Instance {
	t.Helper()
	rows, err := srv.Items.ListByOwner(petCtx(), ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, row := range rows {
		if row.ObjectID == objectID {
			return row
		}
	}
	t.Fatalf("no persisted item row for object %d (owner %d)", objectID, ownerID)
	return nil
}

func assertPetPickupPlainParams(t *testing.T, frame []byte, ownerName string, templateID int32) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // message id already matched
	if params := r.ReadInt32(); params != 2 {
		t.Fatalf("param count = %d, want 2", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("param 1 type = %d, want text", typ)
	}
	if got := r.ReadString(); got != ownerName {
		t.Fatalf("owner name = %q, want %q", got, ownerName)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
		t.Fatalf("param 2 type = %d, want item name", typ)
	}
	if got := r.ReadInt32(); got != templateID {
		t.Fatalf("template id = %d, want %d", got, templateID)
	}
}

func assertPetPickupEnchantParams(t *testing.T, frame []byte, ownerName string, enchant, templateID int32) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // message id already matched
	if params := r.ReadInt32(); params != 3 {
		t.Fatalf("param count = %d, want 3", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("param 1 type = %d, want text", typ)
	}
	if got := r.ReadString(); got != ownerName {
		t.Fatalf("owner name = %q, want %q", got, ownerName)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
		t.Fatalf("param 2 type = %d, want number", typ)
	}
	if got := r.ReadInt32(); got != enchant {
		t.Fatalf("enchant = %d, want %d", got, enchant)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
		t.Fatalf("param 3 type = %d, want item name", typ)
	}
	if got := r.ReadInt32(); got != templateID {
		t.Fatalf("template id = %d, want %d", got, templateID)
	}
}
