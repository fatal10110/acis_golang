package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// Reference: a pet that keeps the ground item it picked up stores it with
// Pet.addItem(item, true) (SummonAI.thinkPickUp for a partyless owner,
// Party.distributeItem(Player, ItemInstance, Summon) when the loot rule picks
// the owner). With sendMessage set, Pet.java:151-167 sends, through
// Summon.sendPacket (Summon.java:314-318) to the owner alone, after the
// attention broadcast and before the inventory add:
//
//	adena            PET_PICKED_S1_ADENA (1023): item-number count
//	enchant > 0      PET_PICKED_S1_S2    (1022): number enchant, item-name
//	count > 1        PET_PICKED_S2_S1_S  (1021): item-name, item-number count
//	else             PET_PICKED_S1       (1020): item-name
//
// Parameter types are SystemMessage.java's: 1 number, 3 item name, 6 item
// number.
const (
	petPickedS1      = 1020
	petPickedS2S1S   = 1021
	petPickedS1S2    = 1022
	petPickedS1Adena = 1023

	smParamNumber     = int32(1)
	smParamItemName   = int32(3)
	smParamItemNumber = int32(6)
)

// smParam is one decoded SystemMessage parameter of a non-text type.
type smParam struct {
	Type, Value int32
}

// petPickedLines returns every PET_PICKED_* line in frames with its decoded
// parameters, keyed by message id.
func petPickedLines(t *testing.T, frames [][]byte) map[int][][]smParam {
	t.Helper()
	out := map[int][][]smParam{}
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		id := int(r.ReadInt32())
		if id < petPickedS1 || id > petPickedS1Adena {
			continue
		}
		var params []smParam
		for range r.ReadInt32() {
			typ := r.ReadInt32()
			if typ == serverpackets.SystemMessageParamText {
				t.Fatalf("PET_PICKED %d carries a text parameter", id)
			}
			params = append(params, smParam{Type: typ, Value: r.ReadInt32()})
		}
		out[id] = append(out[id], params)
	}
	return out
}

// requirePetPicked requires frames to hold exactly one PET_PICKED_* line,
// message id with params.
func requirePetPicked(t *testing.T, frames [][]byte, id int, params ...smParam) {
	t.Helper()
	lines := petPickedLines(t, frames)
	if len(lines) != 1 || len(lines[id]) != 1 || !slices.Equal(lines[id][0], params) {
		t.Fatalf("PET_PICKED lines = %v, want one %d with %v", lines, id, params)
	}
}

// requireNoPetPicked requires frames to hold no PET_PICKED_* line.
func requireNoPetPicked(t *testing.T, who string, frames [][]byte) {
	t.Helper()
	if lines := petPickedLines(t, frames); len(lines) != 0 {
		t.Fatalf("%s heard PET_PICKED lines %v, want none", who, lines)
	}
}

// requireOpcodes requires frames to carry exactly want, in order.
func requireOpcodes(t *testing.T, what string, frames [][]byte, want ...byte) {
	t.Helper()
	if got := frameOpcodes(frames); !slices.Equal(got, want) {
		t.Fatalf("%s frames = %x, want %x", what, got, want)
	}
}

// TestPetPickedAdena: a partyless owner's wolf loots 40 adena and keeps it;
// the owner reads 1023 with the looted count, then the PetInventoryUpdate.
func TestPetPickedAdena(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)
	groundID := h.seedGroundNearOwner(t, item.AdenaID, 40)

	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	requireOpcodes(t, "adena pickup", rest, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate)
	requirePetPicked(t, rest, petPickedS1Adena, smParam{smParamItemNumber, 40})
}

// TestPetPickedStack: a stack of five potions reads 1021, item name then
// count.
func TestPetPickedStack(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)
	groundID := h.seedGroundNearOwner(t, partyPotionID, 5)

	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	requireOpcodes(t, "stack pickup", rest, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate)
	requirePetPicked(t, rest, petPickedS2S1S, smParam{smParamItemName, partyPotionID}, smParam{smParamItemNumber, 5})
}

// TestPetPickedSingle: one plain weapon reads 1020 with its name alone,
// after the owner-named attention line (SummonAI.java:214-222) and before
// the PetInventoryUpdate.
func TestPetPickedSingle(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)
	groundID := h.seedGroundNearOwner(t, 30, 1)

	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	requireOpcodes(t, "single pickup", rest,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate)
	if id := systemMessageID(t, rest[0]); id != serverpackets.SystemMessageAttentionS1PetPickedUpS2 {
		t.Fatalf("first line after the loot broadcast = %d, want the attention 1535", id)
	}
	requirePetPicked(t, rest[1:2], petPickedS1, smParam{smParamItemName, 30})
}

// TestPetPickedEnchanted: a +7 weapon reads 1022, enchant level then item
// name, after the enchanted attention line 1536.
func TestPetPickedEnchanted(t *testing.T) {
	t.Parallel()
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
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeRequestDropItem(weaponID, 1, int32(x+20), int32(y), int32(z)))
	readUntilOpcode(t, h.client, serverpackets.OpcodeDropItem, "DropItem")
	h.settleInventoryUpdates(t)

	rest := requirePickupHead(t, h.petPickup(t, weaponID), wolf, weaponID)
	requireOpcodes(t, "enchanted pickup", rest,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate)
	if id := systemMessageID(t, rest[0]); id != serverpackets.SystemMessageAttentionS1PetPickedUpS2S3 {
		t.Fatalf("first line after the loot broadcast = %d, want the attention 1536", id)
	}
	requirePetPicked(t, rest[1:2], petPickedS1S2, smParam{smParamNumber, 7}, smParam{smParamItemName, 30})
}

// TestPetPickedHerbSilent: a herb the pet uses instead of keeping goes
// through ItemHandler, never Pet.addItem, so no PET_PICKED line follows.
func TestPetPickedHerbSilent(t *testing.T) {
	t.Parallel()
	h, wolf := bootHerbWolf(t)
	groundID := h.seedGroundNearOwner(t, petHerbID, 1)

	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	requireNoPetPicked(t, "owner", rest)
}

// TestPartyPetPickedOnlyForTheOwnersTurn: under a by-turn party rule the
// first pickup goes to Mate, so the owner hears Mate obtain it and no
// PET_PICKED line; the next is the owner's turn, the wolf keeps it, and the
// owner alone reads 1021 for it, before the PetInventoryUpdate.
func TestPartyPetPickedOnlyForTheOwnersTurn(t *testing.T) {
	t.Parallel()
	h, wolf, mate, _ := partyPet(t, party.LootByTurn)

	ownerFrames := h.petPickup(t, h.seedGroundNearOwner(t, partyPotionID, 5))
	requireNoPetPicked(t, "owner on Mate's turn", ownerFrames)
	requireNoPetPicked(t, "Mate on Mate's turn", drainFrames(t, mate))

	groundID := h.seedGroundNearOwner(t, partyPotionID, 2)
	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	requireOpcodes(t, "owner's-turn pickup", rest, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePetInventoryUpdate)
	requirePetPicked(t, rest, petPickedS2S1S, smParam{smParamItemName, partyPotionID}, smParam{smParamItemNumber, 2})
	requireNoPetPicked(t, "Mate on the owner's turn", drainFrames(t, mate))
}

// TestPartyPetPickedNotForSharedAdena: adena under a party is shared out and
// the pet keeps none, so nobody reads 1023.
func TestPartyPetPickedNotForSharedAdena(t *testing.T) {
	t.Parallel()
	h, _, mate, _ := partyPet(t, party.LootFindersKeepers)

	requireNoPetPicked(t, "owner", h.petPickup(t, h.seedGroundNearOwner(t, item.AdenaID, 100)))
	requireNoPetPicked(t, "Mate", drainFrames(t, mate))
}
