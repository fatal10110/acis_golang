package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// TestRespawnRestoresQueuedReturnSave returns a pet while the collar's
// persistence lane is backed up, so the pets-row write is still queued, and
// summons it again with the lane still held. The summon must neither wait
// for the lane nor read the stale row: it restores the state the return
// queued, so the gained exp survives.
func TestRespawnRestoresQueuedReturnSave(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	pet.AddExpAndSp(100, 0)
	wantExp := pet.Exp()

	h.srv.HoldPersistenceLane(t, h.collarID)
	h.client.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	drainUntilQuiet(t, h.client)
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row while the lane is held: ok=%v err=%v, want no row yet", ok, err)
	}

	respawned, _ := h.spawnWolf(t)
	if got := respawned.Exp(); got != wantExp {
		t.Fatalf("respawned pet Exp() = %d, want the queued save's %d", got, wantExp)
	}
}

// TestRenameLandsAfterQueuedPetSave queues an autosave of an unnamed pet
// behind the collar's held lane, then renames it. The rename's pets-row write must land
// after the queued autosave write, so the row keeps the new name.
func TestRenameLandsAfterQueuedPetSave(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	release := h.srv.HoldPersistenceLane(t, h.collarID)
	h.srv.QueueAutosave()
	h.client.Send(encodeRequestChangePetName("Fenrir"))
	drainFrames(t, h.client)
	release()
	h.srv.FlushPersistence(t)

	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name = %q, want Fenrir", state.Name)
	}
}

// TestCollarTradeKeepsPetsRowWritesOrdered returns a pet while its first
// owner's persistence lane is backed up, trades the collar to a second
// player, who summons the pet, gains exp and returns it. Once the first
// owner's lane drains, the pets row must hold the second owner's newer exp:
// every write of one pets row stays ordered even though the collar changed
// hands between them, as the reference's synchronous store on unsummon
// guarantees.
func TestCollarTradeKeepsPetsRowWritesOrdered(t *testing.T) {
	h := bootOwnerWithCollar(t)
	buyerID := h.srv.SeedCharacterFor(t, "player2", "Buyer", 1, 0).ID
	if buyerID%persist.Lanes == h.ownerID%persist.Lanes {
		t.Fatalf("owner %d and buyer %d share a persistence lane; the scenario needs distinct lanes", h.ownerID, buyerID)
	}
	buyer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, buyer)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer)

	pet, _ := h.spawnWolf(t)
	pet.AddExpAndSp(100, 0)

	release := h.srv.HoldPersistenceLane(t, h.ownerID)
	h.client.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeTradeRequest(buyerID))
	drainUntilQuiet(t, buyer)
	buyer.Send(encodeAnswerTradeRequest(1))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer)
	h.client.Send(encodeAddTradeItem(0, h.collarID, 1))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer)
	h.client.Send(encodeTradeDone(1))
	drainUntilQuiet(t, buyer)
	buyer.Send(encodeTradeDone(1))
	waitFor(t, "collar in the buyer's inventory", func() bool {
		obj, ok := h.srv.State.Player(buyerID)
		if !ok {
			return false
		}
		return obj.(interface {
			Inventory() *itemcontainer.Inventory
		}).Inventory().ItemByObjectID(h.collarID) != nil
	})
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer)

	buyer.Send(encodeUseItem(h.collarID, false))
	var buyerPet *summon.Actor
	waitFor(t, "buyer's pet in world state", func() bool {
		obj, ok := h.srv.State.Summon(buyerID)
		if ok {
			buyerPet, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drainUntilQuiet(t, buyer)
	buyerPet.AddExpAndSp(150, 0)
	wantExp := buyerPet.Exp()
	buyer.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, buyer, serverpackets.OpcodePetDelete, "buyer PetDelete")
	drainUntilQuiet(t, buyer)

	release()
	h.srv.FlushPersistence(t)
	if got := h.savedPetState(t).Exp; got != wantExp {
		t.Fatalf("pets row exp = %d, want the buyer's later save %d", got, wantExp)
	}
}

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeAnswerTradeRequest(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeAddTradeItem(tradeID, objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAddTradeItem)
	w.WriteInt32(tradeID)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeTradeDone(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeDone)
	w.WriteInt32(response)
	return w.Bytes()
}
