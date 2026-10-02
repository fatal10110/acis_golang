package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: SummonAI's pet pickup hands a partied owner's loot to
// Party.distributeItem(owner, item, pet): adena is split among the members
// in party range of the owner and the pet keeps none; anything else goes
// to the looter the rule picks, into the pet's inventory when that is the
// owner, silently, or else to the member, whom the others hear take it
// (S1_OBTAINED_S3_S2, 299).

const partyPotionID int32 = 20

// partyPet boots the owner with its wolf out and the partyless Mate, then
// forms a party of the two under rule, the owner leading.
func partyPet(t *testing.T, rule party.LootRule) (*petWorld, *summon.Actor, *testsupport.ScriptedClient, int32) {
	t.Helper()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	mateID := h.srv.SeedCharacterFor(t, "player2", "Mate", 1, 0).ID
	mate := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, mate)

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString("Mate")
	w.WriteInt32(int32(rule))
	h.client.Send(w.Bytes())
	drainUntilQuiet(t, h.client)
	answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	answer.WriteInt32(1)
	mate.Send(answer.Bytes())
	drainUntilQuiet(t, mate)
	drainUntilQuiet(t, h.client)
	return h, wolf, mate, mateID
}

// partyMessages returns the SystemMessage ids in frames with their
// parameters, text as a string and anything else as an int32.
func partyMessages(frames [][]byte) map[int32][][]any {
	out := map[int32][][]any{}
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		id := r.ReadInt32()
		var params []any
		for range r.ReadInt32() {
			if r.ReadInt32() == serverpackets.SystemMessageParamText {
				params = append(params, r.ReadString())
			} else {
				params = append(params, r.ReadInt32())
			}
		}
		out[id] = append(out[id], params)
	}
	return out
}

func requirePartyMessage(t *testing.T, who string, frames [][]byte, id int, params ...any) {
	t.Helper()
	got := partyMessages(frames)[int32(id)]
	if len(got) != 1 || !slices.Equal(got[0], params) {
		t.Fatalf("%s: messages %d = %v, want one with %v", who, id, got, params)
	}
}

// TestPartyPetPickupFollowsTheTurn: the first pickup goes to Mate, whose
// turn comes first, and the owner hears Mate take it; the next turn is the
// owner's, so the stack goes into the wolf's inventory and nobody hears of
// it.
func TestPartyPetPickupFollowsTheTurn(t *testing.T) {
	t.Parallel()
	h, wolf, mate, mateID := partyPet(t, party.LootByTurn)

	ownerFrames := h.petPickup(t, h.seedGroundNearOwner(t, partyPotionID, 5))
	mateFrames := drainFrames(t, mate)
	if got := h.srv.PlayerInventory(t, mateID).ItemCount(partyPotionID, -1, true); got != 5 {
		t.Fatalf("Mate carries %d potions, want the 5 the wolf picked up", got)
	}
	if got := wolf.PetInventory().ItemCount(partyPotionID, -1, true); got != 0 {
		t.Fatalf("the wolf carries %d potions, want none", got)
	}
	requirePartyMessage(t, "owner", ownerFrames, serverpackets.SystemMessageS1ObtainedS3S2, "Mate", partyPotionID, int32(5))
	requirePartyMessage(t, "Mate", mateFrames, serverpackets.SystemMessageYouPickedUpS2S1, partyPotionID, int32(5))

	ownerFrames = h.petPickup(t, h.seedGroundNearOwner(t, partyPotionID, 2))
	mateFrames = drainFrames(t, mate)
	if got := wolf.PetInventory().ItemCount(partyPotionID, -1, true); got != 2 {
		t.Fatalf("the wolf carries %d potions, want the 2 of the owner's turn", got)
	}
	if got := h.srv.PlayerInventory(t, h.ownerID).ItemCount(partyPotionID, -1, true); got != 0 {
		t.Fatalf("the owner carries %d potions, want none: the wolf keeps its turn's loot", got)
	}
	for who, frames := range map[string][][]byte{"owner": ownerFrames, "Mate": mateFrames} {
		if got := partyMessages(frames)[serverpackets.SystemMessageS1ObtainedS3S2]; len(got) != 0 {
			t.Fatalf("%s heard %v of the wolf's own loot", who, got)
		}
	}
}

// TestPartyPetPickupSplitsAdena: adena the wolf picks up is split between
// the owner and Mate; the wolf keeps none.
func TestPartyPetPickupSplitsAdena(t *testing.T) {
	t.Parallel()
	h, wolf, mate, mateID := partyPet(t, party.LootFindersKeepers)

	ownerFrames := h.petPickup(t, h.seedGroundNearOwner(t, item.AdenaID, 100))
	mateFrames := drainFrames(t, mate)
	for who, id := range map[string]int32{"owner": h.ownerID, "Mate": mateID} {
		if got := h.srv.PlayerInventory(t, id).ItemCount(item.AdenaID, -1, true); got != 50 {
			t.Fatalf("%s carries %d adena, want 50", who, got)
		}
	}
	if got := wolf.PetInventory().ItemCount(item.AdenaID, -1, true); got != 0 {
		t.Fatalf("the wolf carries %d adena, want none", got)
	}
	requirePartyMessage(t, "owner", ownerFrames, serverpackets.SystemMessageEarnedS1Adena, int32(50))
	requirePartyMessage(t, "Mate", mateFrames, serverpackets.SystemMessageEarnedS1Adena, int32(50))
	if len(h.srv.GroundItems.Snapshots(nil)) != 0 {
		t.Fatal("the shared adena is still on the ground")
	}
}
