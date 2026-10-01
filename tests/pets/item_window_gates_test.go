package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestGiveItemToPet.runImpl (RequestGiveItemToPet.java:33-50)
// refuses, in order, a chaotic owner while KarmaPlayerCanTrade is off (S1
// "You cannot trade in a chaotic state."), an owner with a store set up
// (CANNOT_PICKUP_OR_USE_ITEM_WHILE_TRADING) and an owner tied up in a trade
// (ALREADY_TRADING). RequestGetItemFromPet.runImpl (:32-36) refuses the
// latter with ALREADY_TRADING before it cancels the enchant selection.
// Player.checkItemManipulation (Player.java:2073) and
// Pet.checkItemManipulation (Pet.java:481-485) move nothing, silently, for
// a count below one, above the stack, or above one on an unstackable item.

// chaoticRefusal is the S1 text a chaotic owner's give is refused with.
const chaoticRefusal = "You cannot trade in a chaotic state."

// tunicID is the shared catalog's unstackable, tradable tunic.
const tunicID = int32(40)

// bootOwnerWithKarma is bootOwnerWithCollarOpts for an owner that enters
// the world carrying karma, with 100 adena, and calls out its wolf.
func bootOwnerWithKarma(t *testing.T, karma int, opts ...gameservertest.Option) *petWorld {
	t.Helper()
	srv := bootPets(t, opts...)
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = ? WHERE obj_Id = ?", karma, ownerID); err != nil {
		t.Fatalf("set karma: %v", err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: srv.GiveItem(t, ownerID, wolfCollarID, 1), seeded: map[int32][]int32{}}
	h.seeded[item.AdenaID] = []int32{srv.GiveItem(t, ownerID, item.AdenaID, 100)}
	startInWorld(t, h.client)
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	return h
}

// requireOnlyRefusal requires frames to be the one static system message.
func requireOnlyRefusal(t *testing.T, frames [][]byte, messageID int, what string) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("%s frames = %x, want only system message %d", what, frameOpcodes(frames), messageID)
	}
	assertStaticSystemMessage(t, frames[0], messageID)
}

// TestGiveItemToPetKarmaGate pins the KarmaPlayerCanTrade switch on the
// hand-over: with the switch off a chaotic owner reads the chaotic-state
// text alone and nothing moves; a lawful owner, or a chaotic one under the
// shipped switch, hands the adena over.
func TestGiveItemToPetKarmaGate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		karma   int
		opts    []gameservertest.Option
		refused bool
	}{
		{"chaotic, switch off", 240, []gameservertest.Option{gameservertest.WithKarmaTrade(false)}, true},
		{"lawful, switch off", 0, []gameservertest.Option{gameservertest.WithKarmaTrade(false)}, false},
		{"chaotic, switch on", 240, []gameservertest.Option{gameservertest.WithKarmaTrade(true)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithKarma(t, tc.karma, tc.opts...)
			adena := h.seededItem(t, item.AdenaID)
			if !tc.refused {
				h.giveToPet(t, adena, 30)
				if got := h.collarItemCount(t, item.AdenaID); got != 30 {
					t.Fatalf("pet adena = %d, want 30", got)
				}
				return
			}
			frames := h.sendAndTick(t, encodeRequestGiveItemToPet(adena, 30))
			if len(frames) != 1 {
				t.Fatalf("chaotic give frames = %x, want only the chaotic-state text", frameOpcodes(frames))
			}
			assertSystemMessageText(t, frames[0], serverpackets.SystemMessageS1, chaoticRefusal)
			if got := h.ownerItemCount(t, item.AdenaID); got != 100 {
				t.Fatalf("owner adena = %d, want all 100 kept", got)
			}
			if got := h.collarItemCount(t, item.AdenaID); got != 0 {
				t.Fatalf("pet adena = %d, want none", got)
			}
		})
	}
}

// TestPetItemWindowRefusedWhileTrading pins the trade gates of both
// directions: with a trade request pending or a trade window open, a give
// and a take-back are each refused with ALREADY_TRADING alone and nothing
// moves; with a store set up, a give is refused with
// CANNOT_PICKUP_OR_USE_ITEM_WHILE_TRADING.
func TestPetItemWindowRefusedWhileTrading(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		setup     func(t *testing.T, h *petWorld, buyer secondPlayer)
		giveMsg   int
		checkTake bool
	}{
		{"trade request pending", func(t *testing.T, h *petWorld, buyer secondPlayer) {
			h.client.Send(encodeTradeRequest(buyer.id))
			readUntilOpcode(t, buyer.client, serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
			drainUntilQuiet(t, h.client)
		}, serverpackets.SystemMessageAlreadyTrading, true},
		{"trade window open", func(t *testing.T, h *petWorld, buyer secondPlayer) {
			openTrade(t, h, buyer)
		}, serverpackets.SystemMessageAlreadyTrading, true},
		{"store set up", func(t *testing.T, h *petWorld, _ secondPlayer) {
			h.srv.SetPlayerOperating(t, h.ownerID, true)
		}, serverpackets.SystemMessageCannotPickupOrUseItemTrading, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 100})
			buyer := h.joinSecondPlayer(t, "Buyer")
			h.spawnWolf(t)
			h.settleInventoryUpdates(t)
			adena := h.seededItem(t, item.AdenaID)
			h.giveToPet(t, adena, 30)
			petAdena := h.petStack(t, item.AdenaID)

			tc.setup(t, h, buyer)
			requireOnlyRefusal(t, h.sendAndTick(t, encodeRequestGiveItemToPet(adena, 10)), tc.giveMsg, "give")
			if tc.checkTake {
				requireOnlyRefusal(t, h.sendAndTick(t, encodeRequestGetItemFromPet(petAdena, 10)), serverpackets.SystemMessageAlreadyTrading, "take-back")
			}
			if got := h.ownerItemCount(t, item.AdenaID); got != 70 {
				t.Fatalf("owner adena = %d, want 70", got)
			}
			if got := h.collarItemCount(t, item.AdenaID); got != 30 {
				t.Fatalf("pet adena = %d, want 30", got)
			}
		})
	}
}

// TestPetTransferRejectsInvalidCount pins the count integrity of both
// directions with a scroll selection active: a give of more adena than the
// owner holds, a give of two units of an unstackable tunic, and a take-back
// of more adena than the pet carries each answer the enchant cancel pair
// alone, and nothing moves.
func TestPetTransferRejectsInvalidCount(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t,
		seedItem{TemplateID: item.AdenaID, Count: 100},
		seedItem{TemplateID: tunicID, Count: 1},
		seedItem{TemplateID: enchantScrollID, Count: 3})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	adena := h.seededItem(t, item.AdenaID)
	h.giveToPet(t, adena, 30)
	petAdena := h.petStack(t, item.AdenaID)

	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"give above the stack", encodeRequestGiveItemToPet(adena, 71)},
		{"give two unstackable", encodeRequestGiveItemToPet(h.seededItem(t, tunicID), 2)},
		{"take-back above the stack", encodeRequestGetItemFromPet(petAdena, 31)},
	} {
		h.selectEnchantScroll(t)
		if rest := requireEnchantCancelled(t, h.sendAndTick(t, tc.payload)); len(rest) != 0 {
			t.Fatalf("%s: frames after the cancel = %x, want none", tc.name, frameOpcodes(rest))
		}
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 70 {
		t.Fatalf("owner adena = %d, want 70", got)
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 30 {
		t.Fatalf("pet adena = %d, want 30", got)
	}
	if got := h.ownerItemCount(t, tunicID); got != 1 {
		t.Fatalf("owner tunics = %d, want the tunic kept", got)
	}
	if got := h.collarItemCount(t, tunicID); got != 0 {
		t.Fatalf("pet tunics = %d, want none", got)
	}
}

// petStack returns the object id of the pet's stack of templateID.
func (h *petWorld) petStack(t *testing.T, templateID int32) int32 {
	t.Helper()
	obj, ok := h.srv.State.Summon(h.ownerID)
	if !ok {
		t.Fatal("pet missing from world state")
	}
	pet, ok := obj.(*summon.Actor)
	if !ok || pet.PetInventory() == nil {
		t.Fatalf("summon %T has no pet inventory", obj)
	}
	inst := pet.PetInventory().ItemByTemplateID(templateID)
	if inst == nil {
		t.Fatalf("pet holds no template %d", templateID)
	}
	return inst.ObjectID
}
