package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// petPickupReach is how close a pet comes to a ground item before it
	// takes it (strict).
	petPickupReach = 36
	// stunSkillID is the skill whose Stun effect lands on the pet.
	stunSkillID = 92
	// boundTokenID is a stackable item that cannot be traded, dropped or
	// destroyed: a pet may not be handed one, but picks it up.
	boundTokenID = int32(9640)
	// groundArrowID is the shared catalog's wooden arrow.
	groundArrowID = int32(17)
)

// seedGroundAt puts count of templateID on the ground at x, y, z, loot-owned
// by the owner, and returns its object id.
func (h *petWorld) seedGroundAt(t *testing.T, templateID, count int32, x, y, z int) int32 {
	t.Helper()
	known := map[int32]bool{}
	for _, snap := range h.srv.GroundItems.Snapshots(nil) {
		known[snap.ObjectID] = true
	}
	h.srv.SeedGroundItem(t, h.ownerID, templateID, count, x, y, z)
	drainUntilQuiet(t, h.client)
	for _, snap := range h.srv.GroundItems.Snapshots(nil) {
		if !known[snap.ObjectID] {
			return snap.ObjectID
		}
	}
	t.Fatalf("seeded ground item %d not tracked", templateID)
	return 0
}

// petOntoGround stands the owner's pet on the ground item groundID, so a
// pickup command takes it without walking first.
func (h *petWorld) petOntoGround(t *testing.T, groundID int32) {
	t.Helper()
	obj, ok := h.srv.State.Summon(h.ownerID)
	if !ok {
		t.Fatal("owner has no summon")
	}
	pet, ok := obj.(*summon.Actor)
	if !ok {
		t.Fatalf("summon %T is not a summon actor", obj)
	}
	obj, ok = h.srv.State.Object(groundID)
	ground, isGround := obj.(*grounditem.Item)
	if !ok || !isGround {
		t.Fatalf("ground item %d not in the world", groundID)
	}
	x, y, z := ground.Position()
	movePet(t, pet, location.Location{X: x, Y: y, Z: z})
	drainUntilQuiet(t, h.client)
}

// TestPetWalksToFarItemBeforePickingItUp commands the wolf to loot adena 150
// units away. The pet does not take it at once: it walks straight to it,
// and only once within reach takes it into its inventory, row included.
func TestPetWalksToFarItemBeforePickingItUp(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	x, y, z := wolf.Position()
	spot := location.Location{X: x + 150, Y: y, Z: z}
	groundID := h.seedGroundAt(t, item.AdenaID, 40, spot.X, spot.Y, spot.Z)

	h.client.Send(encodeRequestPetGetItem(groundID))
	frames := syncFrames(t, h.client)
	if frameIndex(frames, serverpackets.OpcodeMoveToLocation, wolf.ObjectID()) < 0 {
		t.Fatalf("pickup command frames = %x, want the pet's MoveToLocation", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodeGetItem) || hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("pickup command frames = %x, want neither an instant pickup nor a refusal", frameOpcodes(frames))
	}
	if _, ok := h.srv.State.Object(groundID); !ok {
		t.Fatal("ground adena left the world before the pet reached it")
	}

	h.srv.AdvanceUntil(t, "pet pickup after its walk", func() bool {
		_, ok := h.srv.State.Object(groundID)
		return !ok
	})
	at := wolf.Move().Position()
	if d := at.Distance3D(spot); d >= petPickupReach {
		t.Fatalf("pet took the adena %.1f units away, want within %d", d, petPickupReach)
	}
	if got := h.petRowCount(t, item.AdenaID); got != 40 {
		t.Fatalf("pet adena rows = %d, want 40", got)
	}
}

// TestStunnedPetRefusesPickupAndOrders stuns the wolf beside a ground item.
// A stunned pet is out of its owner's control: the pickup command answers
// ActionFailed alone and the item stays, and the Stop command reads
// PET_REFUSING_ORDER.
func TestStunnedPetRefusesPickupAndOrders(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	x, y, z := wolf.Position()
	groundID := h.seedGroundAt(t, item.AdenaID, 40, x+10, y, z)
	landFearPetEffect(t, wolf, wolf, "Stun", stunSkillID, 1, 30)
	drainUntilQuiet(t, h.client)
	if !wolf.OutOfControl() {
		t.Fatal("OutOfControl() = false for a stunned pet, want true")
	}

	h.client.Send(encodeRequestPetGetItem(groundID))
	requireOpcodes(t, "stunned pet pickup", syncFrames(t, h.client), serverpackets.OpcodeActionFailed)
	h.srv.Settle(t)
	if _, ok := h.srv.State.Object(groundID); !ok {
		t.Fatal("stunned pet picked the adena up")
	}
	if got := h.petRowCount(t, item.AdenaID); got != 0 {
		t.Fatalf("pet adena rows = %d, want none", got)
	}

	h.client.Send(encodeRequestActionUse(petStopAction, false))
	frames := syncFrames(t, h.client)
	if !hasSystemMessage(frames, serverpackets.SystemMessagePetRefusingOrder) {
		t.Fatalf("stunned pet Stop frames = %x, want PET_REFUSING_ORDER", frameOpcodes(frames))
	}
}

// TestPetPickupRefusesOnlyAmmunition loots, beside the wolf, an item no pet
// may be handed (it cannot be traded, dropped or destroyed) and then arrows.
// A pet's pickup refuses only arrows and shots: the bound token reaches its
// inventory, while the arrows answer ITEM_NOT_FOR_PETS and stay put.
func TestPetPickupRefusesOnlyAmmunition(t *testing.T) {
	t.Parallel()
	catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: boundTokenID, Name: "Bound Token", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		EtcItem: &item.EtcItemDetail{Type: item.EtcItemNone},
	}))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(catalog)})
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	x, y, z := wolf.Position()

	token := h.seedGroundAt(t, boundTokenID, 3, x+10, y, z)
	requirePickupHead(t, h.petPickup(t, token), wolf, token)
	if got := h.petRowCount(t, boundTokenID); got != 3 {
		t.Fatalf("pet bound tokens = %d, want 3", got)
	}

	arrows := h.seedGroundAt(t, groundArrowID, 20, x+10, y, z)
	frames := h.petPickup(t, arrows)
	if len(frames) != 1 {
		t.Fatalf("arrow pickup frames = %x, want only ITEM_NOT_FOR_PETS", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageItemNotForPets)
	if _, ok := h.srv.State.Object(arrows); !ok {
		t.Fatal("arrows left the ground")
	}
}
