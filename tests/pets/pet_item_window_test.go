package pets

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// armedWolfNPCID is the npc id of the wolf pet, the one pet whose equipment
// is the wolf slot (Pet.canWear / the item's bodypart "wolf").
const armedWolfNPCID = 12077

// petWeaponTemplate is a wolf weapon: a PET-type weapon on the wolf slot, as
// every shipped wolf weapon is.
func petWeaponTemplate() *item.Template {
	return &item.Template{
		ID: fixturePetWeaponID, Name: "Wolf Weapon", Kind: item.KindWeapon, Slot: item.SlotWolf,
		Duration: -1, Dropable: true, Tradable: true, Destroyable: true, DefaultAction: item.ActionEquip,
		Weapon: &item.WeaponDetail{Type: item.WeaponPet},
	}
}

// bootArmedWolf boots the owner with a collar that calls out a wolf able to
// wear wolf-slot equipment, and one wolf weapon already saved in its
// inventory, unequipped. It returns the world, the spawned wolf and the
// weapon's object id.
func bootArmedWolf(t *testing.T) (*petWorld, *summon.Actor, int32) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.ID = armedWolfNPCID
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wolfCollarID, NPCID: armedWolfNPCID, SummonType: 1}})
	if err != nil {
		t.Fatal(err)
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf})),
		gameservertest.WithSummonItems(summonItems),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), petWeaponTemplate()))),
	})
	weapon := item.Instance{ObjectID: h.srv.NewObjectID(), TemplateID: fixturePetWeaponID, OwnerID: h.collarID, Count: 1, Location: item.LocationPet}
	if err := h.srv.Items.Create(context.Background(), h.collarID, weapon); err != nil {
		t.Fatalf("seed pet weapon: %v", err)
	}
	actor, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	return h, actor, weapon.ObjectID
}

// sendAndTick sends payload, waits for the server to run it, then runs one
// inventory-update tick, returning every frame the owner received.
func (h *petWorld) sendAndTick(t *testing.T, payload []byte) [][]byte {
	t.Helper()
	h.client.Send(payload)
	frames := syncFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	return append(frames, drainFrames(t, h.client)...)
}

// TestPetUseItemEquipsWolfWeapon uses a carried wolf weapon from the pet's
// window: the wolf puts it on in its right hand, the owner reads
// PET_PUT_ON_S1 naming it ahead of the PetInventoryUpdate, and the row is
// saved as pet equipment.
func TestPetUseItemEquipsWolfWeapon(t *testing.T) {
	t.Parallel()
	h, wolf, weaponID := bootArmedWolf(t)

	frames := h.sendAndTick(t, encodeRequestPetUseItem(weaponID))
	if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate}; !slices.Equal(got, want) {
		t.Fatalf("equip frames = %x, want PET_PUT_ON_S1 then PetInventoryUpdate", got)
	}
	assertSystemMessageItem(t, frames[0], serverpackets.SystemMessagePetPutOnS1, fixturePetWeaponID)
	if held := wolf.PetInventory().ItemAt(itemcontainer.RHand); held == nil || held.ObjectID != weaponID {
		t.Fatalf("wolf right hand = %+v, want weapon %d", held, weaponID)
	}
	row := mustPersistedItem(t, h.srv, h.collarID, weaponID)
	if row.Location != item.LocationPetEquip || row.LocationData != itemcontainer.RHand {
		t.Fatalf("weapon row = %+v, want pet equipment in the right hand", row)
	}
}

// TestGetItemFromPetUnequipsWornWeapon takes an equipped wolf weapon back.
// The wolf's hand is cleared as the weapon moves to the owner
// (Pet.transferItem's wasWorn), the owner reads PET_TOOK_OFF_S1 naming it,
// and the row lands in the owner's inventory, no longer worn.
func TestGetItemFromPetUnequipsWornWeapon(t *testing.T) {
	t.Parallel()
	h, wolf, weaponID := bootArmedWolf(t)
	h.sendAndTick(t, encodeRequestPetUseItem(weaponID))

	frames := h.sendAndTick(t, encodeRequestGetItemFromPet(weaponID, 1))
	took := findSystemMessage(t, frames, serverpackets.SystemMessagePetTookOffS1)
	if took == nil {
		t.Fatalf("take-back frames = %x, want PET_TOOK_OFF_S1", frameOpcodes(frames))
	}
	assertSystemMessageItem(t, took, serverpackets.SystemMessagePetTookOffS1, fixturePetWeaponID)
	requireInventoryUpdateOrder(t, frames, "take back worn weapon",
		serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodeInventoryUpdate)
	if held := wolf.PetInventory().ItemAt(itemcontainer.RHand); held != nil {
		t.Fatalf("wolf right hand = %+v after the take-back, want empty", held.Snapshot())
	}
	row := mustPersistedItem(t, h.srv, h.ownerID, weaponID)
	if row.Location != item.LocationInventory || row.OwnerID != h.ownerID {
		t.Fatalf("weapon row = %+v, want in the owner's inventory", row)
	}
	if got := h.collarItemCount(t, fixturePetWeaponID); got != 0 {
		t.Fatalf("pet still holds %d weapon rows", got)
	}
}

// TestGiveItemToPetRejectsForbiddenItem hands the wolf arrows, an item type
// pets may never hold. The owner reads ITEM_NOT_FOR_PETS alone and nothing
// moves.
func TestGiveItemToPetRejectsForbiddenItem(t *testing.T) {
	t.Parallel()
	const woodenArrowID = int32(17)
	h := bootOwnerWithCollar(t, seedItem{TemplateID: woodenArrowID, Count: 50})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	arrows := h.seededItem(t, woodenArrowID)

	frames := h.sendAndTick(t, encodeRequestGiveItemToPet(arrows, 10))
	if len(frames) != 1 {
		t.Fatalf("forbidden give frames = %x, want only ITEM_NOT_FOR_PETS", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageItemNotForPets)
	if got := h.ownerItemCount(t, woodenArrowID); got != 50 {
		t.Fatalf("owner arrows = %d, want all 50 kept", got)
	}
	if got := h.collarItemCount(t, woodenArrowID); got != 0 {
		t.Fatalf("pet arrows = %d, want none", got)
	}
}

// enchantScrollID is the shared catalog's weapon enchant scroll.
const enchantScrollID = int32(955)

// selectEnchantScroll uses the scroll from the item window, leaving its
// selection active, and consumes the selection prompt pair.
func (h *petWorld) selectEnchantScroll(t *testing.T) {
	t.Helper()
	h.client.Send(encodeUseItem(h.seededItem(t, enchantScrollID), false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SELECT_ITEM_TO_ENCHANT"), serverpackets.SystemMessageSelectItemToEnchant)
	assertFrameOpcode(t, mustRead(t, h.client, "ChooseInventoryItem"), serverpackets.OpcodeChooseInventoryItem, "ChooseInventoryItem")
}

// requireEnchantCancelled checks frames open with the enchant cancel pair:
// EnchantResult(CANCELLED) then ENCHANT_SCROLL_CANCELLED. It returns the
// frames after them.
func requireEnchantCancelled(t *testing.T, frames [][]byte) [][]byte {
	t.Helper()
	if len(frames) < 2 {
		t.Fatalf("frames = %x, want the enchant cancel pair first", frameOpcodes(frames))
	}
	assertFrameOpcode(t, frames[0], serverpackets.OpcodeEnchantResult, "EnchantResult")
	if got := wire.NewReader(frames[0][1:]).ReadInt32(); got != int32(serverpackets.EnchantResultCancelled) {
		t.Fatalf("EnchantResult = %d, want CANCELLED", got)
	}
	assertStaticSystemMessage(t, frames[1], serverpackets.SystemMessageEnchantScrollCancelled)
	return frames[2:]
}

// TestGiveItemToPetCancelsActiveEnchant gives adena to the wolf with a
// scroll selection active. Once the give has passed its checks the
// selection is cancelled ahead of the transfer (RequestGiveItemToPet's
// cancelActiveEnchant), and the transfer's inventory updates follow.
func TestGiveItemToPetCancelsActiveEnchant(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100}, seedItem{TemplateID: enchantScrollID, Count: 1})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	h.selectEnchantScroll(t)

	rest := requireEnchantCancelled(t, h.sendAndTick(t, encodeRequestGiveItemToPet(h.seededItem(t, item.AdenaID), 30)))
	if got, want := frameOpcodes(rest), []byte{serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodeInventoryUpdate}; !slices.Equal(got, want) {
		t.Fatalf("frames after the cancel = %x, want PetInventoryUpdate then InventoryUpdate", got)
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 30 {
		t.Fatalf("pet adena = %d, want 30", got)
	}
}

// TestGetItemFromPetFailureStillCancelsEnchant takes back an object the
// wolf does not carry with a scroll selection active. RequestGetItemFromPet
// cancels the selection before it resolves the item, so the cancel pair is
// all the owner reads and no inventory changes.
func TestGetItemFromPetFailureStillCancelsEnchant(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100}, seedItem{TemplateID: enchantScrollID, Count: 1})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	h.selectEnchantScroll(t)

	// The owner's own adena stack: an object the pet does not carry.
	frames := h.sendAndTick(t, encodeRequestGetItemFromPet(h.seededItem(t, item.AdenaID), 1))
	if rest := requireEnchantCancelled(t, frames); len(rest) != 0 {
		t.Fatalf("frames after the cancel = %x, want none for a failed take-back", frameOpcodes(rest))
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 100 {
		t.Fatalf("owner adena = %d, want 100 untouched", got)
	}
	if got := h.ownerItemCount(t, enchantScrollID); got != 1 {
		t.Fatalf("owner scrolls = %d, want the cancelled scroll kept", got)
	}
}

// TestDisconnectWithPetOutSettlesCollarAndCarriedItems drops the owner's
// connection with the wolf out and carrying adena. Leaving the world
// unsummons the pet (detachLivePlayer): its row is saved, the collar is
// lifted to the pet's level on the owner's saved row, and what it carried is
// handed to the owner (PetInventory.deleteMe). Coming back and calling the
// wolf out again finds the adena with the owner and none left on the pet.
func TestDisconnectWithPetOutSettlesCollarAndCarriedItems(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithReuseDelays(0, 0)},
		seedItem{TemplateID: item.AdenaID, Count: 100})
	h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 30)

	if err := h.client.Close(); err != nil {
		t.Fatal(err)
	}
	h.srv.AdvanceUntil(t, "owner left world", func() bool {
		_, ok := h.srv.State.Player(h.ownerID)
		return !ok
	})
	h.srv.FlushPersistence(t)

	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || !ok {
		t.Fatalf("pets row after disconnect: ok=%v err=%v", ok, err)
	}
	// Read as the detach wrote it: a later item flush would write a lift
	// the detach missed.
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.ownerID)
	if err != nil {
		t.Fatalf("list owner items: %v", err)
	}
	if i := slices.IndexFunc(rows, func(r *item.Instance) bool { return r.ObjectID == h.collarID }); i < 0 || rows[i].EnchantLevel != wolfLevel {
		t.Fatalf("saved collar after disconnect = %+v, want enchant %d", rows, wolfLevel)
	}

	h.client = h.srv.DialClient(t, h.srv.Account(), 1)
	startInWorld(t, h.client)
	wolf, _ := h.spawnWolf(t)
	if inst := wolf.PetInventory().ItemByTemplateID(item.AdenaID); inst != nil {
		t.Fatalf("called-out wolf still carries %+v", inst.Snapshot())
	}
	if adena := h.srv.PlayerInventory(t, h.ownerID).ItemByTemplateID(item.AdenaID); adena == nil || adena.Snapshot().Count != 100 {
		t.Fatalf("owner adena after coming back = %+v, want 100 (the 30 the pet carried returned)", adena)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 100 {
		t.Fatalf("owner adena rows = %d, want 100", got)
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("pet adena rows = %d, want none", got)
	}
}
