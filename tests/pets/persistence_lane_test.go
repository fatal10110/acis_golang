package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRespawnWaitsForQueuedReturnSave returns a pet while the owner's
// persistence lane is backed up, so the pets-row write is still queued, and
// summons it again. The restore must wait for that write: a restore that
// read the row early would find no row and spawn a fresh pet at the level
// floor instead of the gained exp.
func TestRespawnWaitsForQueuedReturnSave(t *testing.T) {
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	pet.AddExpAndSp(100, 0)
	wantExp := pet.Exp()

	release := h.srv.HoldPersistenceLane(t, h.ownerID)
	h.client.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	drainUntilQuiet(t, h.client)
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row while the lane is held: ok=%v err=%v, want no row yet", ok, err)
	}

	h.client.Send(encodeUseItem(h.collarID, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillLaunched, "collar MagicSkillLaunched")
	// The spawn is parked on the held lane: no PetInfo yet.
	h.client.ExpectNoFrame()
	release()

	var respawned bool
	waitFor(t, "pet in world state", func() bool {
		_, respawned = h.srv.State.Summon(h.ownerID)
		return respawned
	})
	obj, _ := h.srv.State.Summon(h.ownerID)
	if got := obj.(interface{ Exp() int64 }).Exp(); got != wantExp {
		t.Fatalf("respawned pet Exp() = %d, want restored saved value %d", got, wantExp)
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
	// The rename waits for its own write, queued behind the autosave.
	h.client.ExpectNoFrame()
	release()
	drainFrames(t, h.client)
	h.srv.FlushPersistence(t)

	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name = %q, want Fenrir", state.Name)
	}
}
