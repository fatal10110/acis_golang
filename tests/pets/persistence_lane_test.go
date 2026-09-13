package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRespawnRestoresQueuedReturnSave returns a pet while the owner's
// persistence lane is backed up, so the pets-row write is still queued, and
// summons it again with the lane still held. The summon must neither wait
// for the lane nor read the stale row: it restores the state the return
// queued, so the gained exp survives.
func TestRespawnRestoresQueuedReturnSave(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	pet.AddExpAndSp(100, 0)
	wantExp := pet.Exp()

	h.srv.HoldPersistenceLane(t, h.ownerID)
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
// behind a held lane, then renames it. The rename's pets-row write must land
// after the queued autosave write, so the row keeps the new name.
func TestRenameLandsAfterQueuedPetSave(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	release := h.srv.HoldPersistenceLane(t, h.ownerID)
	h.srv.QueueAutosave()
	h.client.Send(encodeRequestChangePetName("Fenrir"))
	drainFrames(t, h.client)
	release()
	h.srv.FlushPersistence(t)

	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name = %q, want Fenrir", state.Name)
	}
}
